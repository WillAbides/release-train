package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"golang.org/x/exp/slog"
	"golang.org/x/mod/modfile"
)

type Runner struct {
	CheckoutDir    string
	Ref            string
	GithubToken    string
	CreateTag      bool
	CreateRelease  bool
	Draft          bool
	V0             bool
	TagPrefix      string
	InitialTag     string
	PrereleaseHook string
	GoModFiles     []string
	Repo           string
	PushRemote     string
	TempDir        string
	ReleaseRefs    []string
	LabelAliases   map[string]string
	CheckPR        int
	GithubClient   GithubClient
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

var modVersionRe = regexp.MustCompile(`v\d+$`)

type Result struct {
	PreviousRef           string          `json:"previous-ref"`
	PreviousVersion       string          `json:"previous-version"`
	FirstRelease          bool            `json:"first-release"`
	ReleaseVersion        *semver.Version `json:"release-version,omitempty"`
	ReleaseTag            string          `json:"release-tag,omitempty"`
	ChangeLevel           ChangeLevel     `json:"change-level"`
	CreatedTag            bool            `json:"created-tag,omitempty"`
	CreatedRelease        bool            `json:"created-release,omitempty"`
	PrereleaseHookOutput  string          `json:"prerelease-hook-output"`
	PrereleaseHookAborted bool            `json:"prerelease-hook-aborted"`
}

func (o *Runner) Next(ctx context.Context) (*Result, error) {
	logger := GetLogger(ctx)
	logger.Debug("starting release Next")
	ref := o.Ref
	if o.Ref == "" {
		ref = "HEAD"
	}
	head, err := RunCmd(o.CheckoutDir, nil, "git", "rev-parse", ref)
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
			ChangeLevel:  ChangeLevelNone,
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

	maxBump := ChangeLevelMajor
	if o.V0 {
		maxBump = ChangeLevelMinor
		if prevVersion.Major() != 0 {
			return nil, fmt.Errorf("v0 flag is set, but previous version %q has major version > 0", prevVersion.String())
		}
	}

	result := Result{
		PreviousRef:     prevRef,
		PreviousVersion: prevVersion.String(),
	}
	var nextRes *GetNextResult
	nextRes, err = GetNext(ctx, &GetNextOptions{
		Repo:         o.Repo,
		GithubClient: o.GithubClient,
		PrevVersion:  prevVersion.String(),
		Base:         prevRef,
		Head:         head,
		MaxBump:      &maxBump,
		LabelAliases: o.LabelAliases,
		CheckPR:      o.CheckPR,
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

func (o *Runner) runGoValidation(modFile string, result *Result) error {
	mfPath := filepath.Join(o.CheckoutDir, filepath.FromSlash(modFile))
	content, err := os.ReadFile(mfPath)
	if err != nil {
		return err
	}
	mf, err := modfile.ParseLax(mfPath, content, nil)
	if err != nil {
		return err
	}
	sv := result.ReleaseVersion
	major := int(sv.Major())
	wantM := ""
	if major > 1 {
		wantM = fmt.Sprintf("v%d", major)
	}
	m := modVersionRe.FindString(mf.Module.Mod.Path)
	if m != wantM {
		return fmt.Errorf("module %s has version suffix %q, want %q", mf.Module.Mod.Path, m, wantM)
	}
	return nil
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
	logger := GetLogger(ctx)
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
	if len(o.ReleaseRefs) > 0 && !gitNameRev(o.CheckoutDir, o.Ref, o.ReleaseRefs) {
		createTag = false
		release = false
	}
	// no tag or release if check-pr is set
	if o.CheckPR != 0 {
		createTag = false
		release = false
	}
	shallow, err := RunCmd(o.CheckoutDir, nil, "git", "rev-parse", "--is-shallow-repository")
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

	if result.ReleaseVersion == nil || !createTag {
		return result, nil
	}
	if !result.FirstRelease && result.PreviousVersion == result.ReleaseVersion.String() {
		logger.Debug("no changes detected since previous release %s, skipping tag", result.PreviousVersion)
		return result, nil
	}

	err = assertTagNotExists(o.CheckoutDir, o.PushRemote, result.ReleaseTag)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(o.assetsDir(), 0o700)
	if err != nil {
		return nil, err
	}

	runEnv := map[string]string{
		"RELEASE_VERSION":    result.ReleaseVersion.String(),
		"RELEASE_TAG":        result.ReleaseTag,
		"PREVIOUS_VERSION":   result.PreviousVersion,
		"FIRST_RELEASE":      fmt.Sprintf("%t", result.FirstRelease),
		"GITHUB_TOKEN":       o.GithubToken,
		"RELEASE_NOTES_FILE": o.releaseNotesFile(),
		"RELEASE_TARGET":     o.releaseTargetFile(),
		"ASSETS_DIR":         o.assetsDir(),
	}

	result.PrereleaseHookOutput, result.PrereleaseHookAborted, err = runPrereleaseHook(ctx, o.CheckoutDir, runEnv, o.PrereleaseHook)
	if err != nil {
		logger.Debug("prerelease hook errored", slog.String("output", result.PrereleaseHookOutput))
		return nil, err
	}
	if result.PrereleaseHookAborted {
		return result, nil
	}

	for _, mf := range o.GoModFiles {
		err = o.runGoValidation(mf, result)
		if err != nil {
			return nil, err
		}
	}

	err = o.tagRelease(result.ReleaseTag)
	if err != nil {
		return nil, err
	}
	teardowns = append(teardowns, func() error {
		_, e := RunCmd(o.CheckoutDir, nil, "git", "push", o.PushRemote, "--delete", result.ReleaseTag)
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

func (o *Runner) tagRelease(releaseTag string) error {
	exists, err := localTagExists(o.CheckoutDir, releaseTag)
	if err != nil {
		return err
	}
	if !exists {
		target := ""
		target, err = o.getReleaseTarget()
		if err != nil {
			return err
		}

		_, err = RunCmd(o.CheckoutDir, nil, "git", "tag", releaseTag, target)
		if err != nil {
			return err
		}
	}
	_, err = RunCmd(o.CheckoutDir, nil, "git", "push", o.PushRemote, releaseTag)
	return err
}

func runPrereleaseHook(ctx context.Context, dir string, env map[string]string, hook string) (stdout string, abort bool, _ error) {
	logger := GetLogger(ctx)
	if hook == "" {
		return "", false, nil
	}
	var stdoutBuf bytes.Buffer
	cmd := exec.Command("sh", "-c", hook)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Stdout = &stdoutBuf
	err := cmd.Run()
	if err != nil {
		logger.Debug("prerelease hook errored", slog.String("output", stdoutBuf.String()))
		exitErr := AsExitErr(err)
		if exitErr != nil {
			err = errors.Join(err, errors.New(string(exitErr.Stderr)))
			logger.Debug("prerelease hook errored", slog.String("stderr", string(exitErr.Stderr)))
			if exitErr.ExitCode() == 10 {
				return stdoutBuf.String(), true, nil
			}
		}
		return "", false, err
	}
	return stdoutBuf.String(), false, nil
}

func gitNameRev(dir, commitish string, refs []string) bool {
	args := []string{"name-rev", commitish, "--no-undefined"}
	for _, ref := range refs {
		args = append(args, "--refs", ref)
	}
	_, err := RunCmd(dir, nil, "git", args...)
	return err == nil
}

// assertTagNotExists returns an error if tag exists either locally or on remote
func assertTagNotExists(dir, remote, tag string) error {
	out, err := RunCmd(dir, nil, "git", "ls-remote", "--tags", remote, tag)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("tag %q already exists on remote", tag)
	}
	ok, err := localTagExists(dir, tag)
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("tag %q already exists locally", tag)
	}
	return nil
}

func localTagExists(dir, tag string) (bool, error) {
	out, err := RunCmd(dir, nil, "git", "tag", "--list", tag)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
