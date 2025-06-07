package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type Runner struct {
	CheckoutDir     string
	Ref             string
	GithubToken     string
	CreateTag       bool
	CreateRelease   bool
	Draft           bool
	V0              bool
	ForcePrerelease bool
	TagPrefix       string
	InitialTag      string
	PreTagHook      string
	Repo            string
	PushRemote      string
	TempDir         string
	ReleaseRefs     []string
	LabelAliases    map[string]string
	CheckPR         int
	GithubClient    GithubClient
	Stdout          io.Writer
	Stderr          io.Writer
}

func (o *Runner) releaseNotesFile() string {
	return filepath.Join(o.TempDir, "release-notes")
}

func (o *Runner) releaseTargetFile() string {
	return filepath.Join(o.TempDir, "release-target")
}

func (o *Runner) assetsDir() string {
	return filepath.Join(o.TempDir, "assets")
}

type Result struct {
	PreviousRef           string          `json:"previous-ref"`
	PreviousVersion       string          `json:"previous-version"`
	FirstRelease          bool            `json:"first-release"`
	ReleaseVersion        *semver.Version `json:"release-version,omitempty"`
	ReleaseTag            string          `json:"release-tag,omitempty"`
	ChangeLevel           changeLevel     `json:"change-level"`
	CreatedTag            bool            `json:"created-tag,omitempty"`
	CreatedRelease        bool            `json:"created-release,omitempty"`
	PrereleaseHookOutput  string          `json:"prerelease-hook-output"`
	PrereleaseHookAborted bool            `json:"prerelease-hook-aborted"`
	PreTagHookOutput      string          `json:"pre-tag-hook-output"`
	PreTagHookAborted     bool            `json:"pre-tag-hook-aborted"`
}

func (o *Runner) Next(ctx context.Context) (*Result, error) {
	logger := getLogger(ctx)
	logger.Debug("starting release Next")
	ref := o.Ref
	if o.Ref == "" {
		ref = "HEAD"
	}
	head, err := runCmd(ctx, &runCmdOpts{
		dir: o.CheckoutDir,
	}, "git", "rev-parse", ref)
	if err != nil {
		return nil, err
	}
	head = strings.TrimSpace(head)
	prevRef, err := getPrevTag(ctx, &getPrevTagOpts{
		Head:     head,
		RepoDir:  o.CheckoutDir,
		Prefixes: []string{o.TagPrefix},
	})
	if err != nil {
		return nil, err
	}
	firstRelease := prevRef == ""
	if firstRelease {
		result := Result{
			FirstRelease: true,
			ReleaseTag:   o.InitialTag,
			ChangeLevel:  changeLevelNone,
		}
		if o.InitialTag != "" {
			result.ReleaseVersion, err = semver.NewVersion(strings.TrimPrefix(o.InitialTag, o.TagPrefix))
			if err != nil {
				return nil, err
			}
		}
		return &result, nil
	}
	prevVersion, err := semver.NewVersion(strings.TrimPrefix(prevRef, o.TagPrefix))
	if err != nil {
		return nil, err
	}

	maxBump := changeLevelMajor
	if o.V0 {
		maxBump = changeLevelMinor
		if prevVersion.Major() != 0 {
			return nil, fmt.Errorf("v0 flag is set, but previous version %q has major version > 0", prevVersion.String())
		}
	}

	result := Result{
		PreviousRef:     prevRef,
		PreviousVersion: prevVersion.String(),
	}
	var nextRes *getNextResult
	nextRes, err = getNext(ctx, &getNextOptions{
		Repo:            o.Repo,
		GithubClient:    o.GithubClient,
		PrevVersion:     prevVersion.String(),
		Base:            prevRef,
		Head:            head,
		MaxBump:         &maxBump,
		LabelAliases:    o.LabelAliases,
		CheckPR:         o.CheckPR,
		ForcePrerelease: o.ForcePrerelease,
	})
	if err != nil {
		return nil, err
	}
	result.ReleaseVersion = &nextRes.NextVersion
	result.ReleaseTag = o.TagPrefix + nextRes.NextVersion.String()
	result.ChangeLevel = nextRes.ChangeLevel
	logger.Debug("returning from release Next", slog.Any("result", result))
	return &result, nil
}

func (o *Runner) repoOwner() string {
	return strings.SplitN(o.Repo, "/", 2)[0]
}

func (o *Runner) repoName() string {
	return strings.SplitN(o.Repo, "/", 2)[1]
}

func (o *Runner) getReleaseTarget() (string, error) {
	targetFile := o.releaseTargetFile()
	_, err := os.Stat(targetFile)
	if err != nil {
		if os.IsNotExist(err) {
			return o.Ref, nil
		}
		return "", err
	}
	content, err := os.ReadFile(o.releaseTargetFile())
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(string(content))
	if target == "" {
		target = o.Ref
	}
	return target, nil
}

func (o *Runner) getReleaseNotes(ctx context.Context, result *Result) (string, error) {
	notesInfo, err := os.Stat(o.releaseNotesFile())
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err == nil && !notesInfo.IsDir() {
		content, e := os.ReadFile(o.releaseNotesFile())
		if e != nil {
			return "", e
		}
		return string(content), nil
	}
	// first release is empty by default
	if result.FirstRelease {
		return "", nil
	}
	return o.GithubClient.GenerateReleaseNotes(ctx, o.repoOwner(), o.repoName(), result.ReleaseTag, result.PreviousRef)
}

func (o *Runner) Run(ctx context.Context) (_ *Result, errOut error) {
	logger := getLogger(ctx)
	logger.Debug("starting Run")
	var teardowns []func() error
	defer func() {
		if errOut == nil {
			return
		}
		for i := len(teardowns) - 1; i >= 0; i-- {
			errOut = errors.Join(errOut, teardowns[i]())
		}
	}()
	createTag := o.CreateTag
	release := o.CreateRelease
	if release {
		createTag = true
	}
	// no tag or release if release-refs is defined and the ref is not in the list
	if len(o.ReleaseRefs) > 0 && !gitNameRev(ctx, o.CheckoutDir, o.Ref, o.ReleaseRefs) {
		createTag = false
		release = false
	}
	// no tag or release if check-pr is set
	if o.CheckPR != 0 {
		createTag = false
		release = false
	}
	shallow, err := runCmd(ctx, &runCmdOpts{
		dir: o.CheckoutDir,
	}, "git", "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, err
	}
	if shallow == "true" {
		return nil, fmt.Errorf("shallow clones are not supported")
	}
	result, err := o.Next(ctx)
	if err != nil {
		return nil, err
	}

	if !result.FirstRelease &&
		result.ReleaseVersion != nil &&
		result.PreviousVersion == result.ReleaseVersion.String() {
		logger.Debug("no changes detected since previous release, skipping tag", slog.String("previous-version", result.PreviousVersion))
		return result, nil
	}

	err = assertTagNotExists(ctx, o.CheckoutDir, o.PushRemote, result.ReleaseTag)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(o.assetsDir(), 0o700)
	if err != nil {
		return nil, err
	}

	*result, err = o.runPreTagHook(ctx, *result)
	if err != nil {
		logger.Debug("runPreTagHook hook errored", slog.String("output", result.PreTagHookOutput))
		return nil, err
	}
	if result.PreTagHookAborted {
		return result, nil
	}
	if result.ReleaseVersion == nil || !createTag {
		return result, nil
	}

	err = o.tagRelease(ctx, result.ReleaseTag)
	if err != nil {
		return nil, err
	}
	teardowns = append(teardowns, func() error {
		_, e := runCmd(ctx, &runCmdOpts{
			dir: o.CheckoutDir,
		}, "git", "push", o.PushRemote, "--delete", result.ReleaseTag)
		return e
	})

	result.CreatedTag = true

	if !release {
		return result, nil
	}

	releaseNotes, err := o.getReleaseNotes(ctx, result)
	if err != nil {
		return nil, err
	}

	prerelease := result.ReleaseVersion.Prerelease() != ""
	rel, err := o.GithubClient.CreateRelease(ctx, o.repoOwner(), o.repoName(), result.ReleaseTag, releaseNotes, prerelease)
	if err != nil {
		return nil, err
	}

	teardowns = append(teardowns, func() error {
		return o.GithubClient.DeleteRelease(ctx, o.repoOwner(), o.repoName(), rel.ID)
	})

	err = o.uploadAssets(ctx, rel.UploadURL)
	if err != nil {
		return nil, err
	}

	result.CreatedRelease = true
	if o.Draft {
		return result, nil
	}

	err = o.GithubClient.PublishRelease(ctx, o.repoOwner(), o.repoName(), rel.ID)
	if err != nil {
		return nil, err
	}

	// push target last because it cannot be easily rolled back
	err = o.pushTarget(ctx)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (o *Runner) uploadAssets(ctx context.Context, uploadURL string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	assets, err := filepath.Glob(filepath.Join(o.assetsDir(), "*"))
	if err != nil {
		return err
	}
	for _, asset := range assets {
		err = o.GithubClient.UploadAsset(ctx, uploadURL, asset)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *Runner) pushTarget(ctx context.Context) error {
	target, err := o.getReleaseTarget()
	if err != nil {
		return err
	}
	ref, err := runCmd(ctx, &runCmdOpts{
		dir: o.CheckoutDir,
	}, "git", "rev-parse", "--verify", "--symbolic-full-name", target)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(ref, "refs/heads/") {
		// only push branches
		return nil
	}
	_, err = runCmd(ctx, &runCmdOpts{
		dir: o.CheckoutDir,
	}, "git", "push", o.PushRemote, target)
	return err
}

func (o *Runner) tagRelease(ctx context.Context, releaseTag string) error {
	exists, err := localTagExists(ctx, o.CheckoutDir, releaseTag)
	if err != nil {
		return err
	}
	if !exists {
		target := ""
		target, err = o.getReleaseTarget()
		if err != nil {
			return err
		}

		_, err = runCmd(ctx, &runCmdOpts{
			dir: o.CheckoutDir,
		}, "git", "tag", releaseTag, target)
		if err != nil {
			return err
		}
	}
	_, err = runCmd(ctx, &runCmdOpts{
		dir: o.CheckoutDir,
	}, "git", "push", o.PushRemote, releaseTag)
	return err
}

func (o *Runner) runPreTagHook(ctx context.Context, result Result) (Result, error) {
	logger := getLogger(ctx)
	if o.PreTagHook == "" {
		return result, nil
	}
	releaseVersion := ""
	if result.ReleaseVersion != nil {
		releaseVersion = result.ReleaseVersion.String()
	}
	env := map[string]string{
		"RELEASE_TAG":        result.ReleaseTag,
		"PREVIOUS_VERSION":   result.PreviousVersion,
		"FIRST_RELEASE":      fmt.Sprintf("%t", result.FirstRelease),
		"GITHUB_TOKEN":       o.GithubToken,
		"RELEASE_NOTES_FILE": o.releaseNotesFile(),
		"RELEASE_TARGET":     o.releaseTargetFile(),
		"ASSETS_DIR":         o.assetsDir(),
		"RELEASE_VERSION":    releaseVersion,
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	var stdout, stderr io.Writer
	stdout = &stdoutBuf
	if o.Stdout != nil {
		stdout = io.MultiWriter(o.Stdout, &stdoutBuf)
	}
	stderr = &stderrBuf
	if o.Stderr != nil {
		stderr = io.MultiWriter(o.Stderr, &stderrBuf)
	}
	_, err := runCmd(ctx, &runCmdOpts{
		dir:    o.CheckoutDir,
		stdout: stdout,
		stderr: stderr,
		env:    env,
	}, "sh", "-c", o.PreTagHook)
	result.PreTagHookOutput = stdoutBuf.String()
	result.PrereleaseHookOutput = stdoutBuf.String()
	if err != nil {
		exitErr := asExitErr(err)
		if exitErr != nil {
			if exitErr.ExitCode() == 10 {
				logger.Debug("pre-tag hook aborted")
				result.PreTagHookAborted = true
				result.PrereleaseHookAborted = true
				return result, nil
			}
			err = exitErr
		}
		logger.Error(
			"pre-tag hook failed",
			slog.Any("err", err),
			slog.String("stdout", stdoutBuf.String()),
			slog.String("stderr", stderrBuf.String()),
		)
		return result, err
	}
	return result, nil
}

func gitNameRev(ctx context.Context, dir, commitish string, refs []string) bool {
	args := []string{"name-rev", commitish, "--no-undefined"}
	for _, ref := range refs {
		args = append(args, "--refs", ref)
	}
	_, err := runCmd(ctx, &runCmdOpts{
		dir: dir,
	}, "git", args...)
	return err == nil
}

// assertTagNotExists returns an error if tag exists either locally or on remote
func assertTagNotExists(ctx context.Context, dir, remote, tag string) error {
	out, err := runCmd(ctx, &runCmdOpts{
		dir: dir,
	}, "git", "ls-remote", "--tags", remote, tag)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("tag %q already exists on remote", tag)
	}
	ok, err := localTagExists(ctx, dir, tag)
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("tag %q already exists locally", tag)
	}
	return nil
}

func localTagExists(ctx context.Context, dir, tag string) (bool, error) {
	out, err := runCmd(ctx, &runCmdOpts{
		dir: dir,
	}, "git", "tag", "--list", tag)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
