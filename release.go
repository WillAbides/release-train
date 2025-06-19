package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	ForceStable     bool
	TagPrefix       string
	InitialTag      string
	PreTagHook      string
	Repo            string
	PushRemote      string
	TempDir         string
	MakeLatest      string
	ReleaseRefs     []string
	LabelAliases    map[string]string
	CheckPR         int
	GithubClient    GithubClient
	Stdout          io.Writer
	Stderr          io.Writer

	ran         bool
	errCleanups []func() error
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

func (o *Runner) cleanupAfterErr() error {
	var err error
	for _, fn := range slices.Backward(o.errCleanups) {
		err = errors.Join(err, fn())
	}
	return err
}

func (o *Runner) addErrCleanup(fn func() error) {
	o.errCleanups = append(o.errCleanups, fn)
}

type Result struct {
	PreviousRef           string          `json:"previous-ref"`
	PreviousVersion       string          `json:"previous-version"`
	PreviousStableRef     string          `json:"previous-stable-ref"`
	PreviousStableVersion string          `json:"previous-stable-version"`
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
	slog.Debug("starting release Next")
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
		Head:      head,
		RepoDir:   o.CheckoutDir,
		TagPrefix: o.TagPrefix,
	})
	if err != nil {
		return nil, err
	}

	// Find the previous stable version
	prevStableRef, err := getPrevTag(ctx, &getPrevTagOpts{
		Head:       head,
		RepoDir:    o.CheckoutDir,
		TagPrefix:  o.TagPrefix,
		StableOnly: true,
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

	// Set the previous stable version
	if prevStableRef != "" {
		var prevStableVersion *semver.Version
		prevStableVersion, err = semver.NewVersion(strings.TrimPrefix(prevStableRef, o.TagPrefix))
		if err != nil {
			return nil, err
		}
		result.PreviousStableVersion = prevStableVersion.String()
		result.PreviousStableRef = prevStableRef
	}

	var nextRes *versionChange
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
		ForceStable:     o.ForceStable,
	})
	if err != nil {
		return nil, err
	}
	result.ReleaseVersion = &nextRes.NextVersion
	result.ReleaseTag = o.TagPrefix + nextRes.NextVersion.String()
	result.ChangeLevel = nextRes.ChangeLevel
	slog.Debug("returning from release Next", slog.Any("result", result))
	return &result, nil
}

func (o *Runner) repoOwner() string {
	owner, _, _ := strings.Cut(o.Repo, "/")
	return owner
}

func (o *Runner) repoName() string {
	_, repo, _ := strings.Cut(o.Repo, "/")
	return repo
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

// shouldCreateTag returns true if a tag should be created.
func (o *Runner) shouldCreateTag(ctx context.Context) bool {
	// only when --create-tag or --create-release is set
	if !cmp.Or(o.CreateTag, o.CreateRelease) {
		return false
	}
	// never create a tag if --check-pr is set
	if o.CheckPR != 0 {
		return false
	}
	// only allowed refs
	return o.isAllowedRef(ctx, o.Ref, o.ReleaseRefs)
}

func (o *Runner) runCmd(ctx context.Context, opts *runCmdOpts, command string, args ...string) (string, error) {
	if opts == nil {
		opts = &runCmdOpts{}
	}
	opts.dir = cmp.Or(opts.dir, o.CheckoutDir)
	return runCmd(ctx, opts, command, args...)
}

func (o *Runner) Run(ctx context.Context) (_ *Result, errOut error) {
	if o.ran {
		panic("Runner.Run called multiple times, this is not allowed")
	}
	o.ran = true
	slog.Debug("starting Run")
	defer func() {
		if errOut != nil {
			errOut = errors.Join(errOut, o.cleanupAfterErr())
		}
	}()
	shallow, err := o.runCmd(ctx, nil, "git", "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, err
	}
	if shallow == "true" {
		return nil, errors.New("shallow clones are not supported")
	}
	result, err := o.Next(ctx)
	if err != nil {
		return nil, err
	}

	if !result.FirstRelease &&
		result.ReleaseVersion != nil &&
		result.PreviousVersion == result.ReleaseVersion.String() {
		slog.Debug("no changes detected since previous release, skipping tag", slog.String("previous-version", result.PreviousVersion))
		return result, nil
	}

	err = o.assertTagNotExists(ctx, o.PushRemote, result.ReleaseTag)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(o.assetsDir(), 0o700)
	if err != nil {
		return nil, err
	}

	*result, err = o.runPreTagHook(ctx, *result)
	if err != nil {
		slog.Debug("runPreTagHook hook errored", slog.String("output", result.PreTagHookOutput))
		return nil, err
	}
	if result.PreTagHookAborted {
		return result, nil
	}
	if result.ReleaseVersion == nil || !o.shouldCreateTag(ctx) {
		return result, nil
	}

	err = o.tagRelease(ctx, result.ReleaseTag)
	if err != nil {
		return nil, err
	}
	o.addErrCleanup(func() error {
		_, e := o.runCmd(ctx, nil, "git", "push", o.PushRemote, "--delete", result.ReleaseTag)
		return e
	})

	result.CreatedTag = true

	if o.CreateRelease {
		err = o.createRelease(ctx, result)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// createRelease handles the creation of a GitHub release, including notes, assets, and publishing.
func (o *Runner) createRelease(ctx context.Context, result *Result) error {
	releaseNotes, err := o.getReleaseNotes(ctx, result)
	if err != nil {
		return err
	}

	prerelease := result.ReleaseVersion.Prerelease() != ""
	rel, err := o.GithubClient.CreateRelease(ctx, o.repoOwner(), o.repoName(), result.ReleaseTag, releaseNotes, prerelease)
	if err != nil {
		return err
	}

	o.addErrCleanup(func() error {
		return o.GithubClient.DeleteRelease(ctx, o.repoOwner(), o.repoName(), rel.ID)
	})

	err = o.uploadAssets(ctx, rel.UploadURL)
	if err != nil {
		return err
	}

	result.CreatedRelease = true
	if o.Draft {
		return nil
	}

	err = o.GithubClient.PublishRelease(ctx, o.repoOwner(), o.repoName(), o.MakeLatest, rel.ID)
	if err != nil {
		return err
	}

	// push target last because it cannot be easily rolled back
	return o.pushTarget(ctx)
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
	ref, err := o.runCmd(ctx, nil, "git", "rev-parse", "--verify", "--symbolic-full-name", target)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(ref, "refs/heads/") {
		// only push branches
		return nil
	}
	_, err = o.runCmd(ctx, nil, "git", "push", o.PushRemote, target)
	return err
}

func (o *Runner) tagRelease(ctx context.Context, releaseTag string) error {
	exists, err := localTagExists(ctx, o.CheckoutDir, releaseTag)
	if err != nil {
		return err
	}
	if !exists {
		var target string
		target, err = o.getReleaseTarget()
		if err != nil {
			return err
		}

		_, err = o.runCmd(ctx, nil, "git", "tag", releaseTag, target)
		if err != nil {
			return err
		}
	}
	_, err = o.runCmd(ctx, nil, "git", "push", o.PushRemote, releaseTag)
	return err
}

func (o *Runner) runPreTagHook(ctx context.Context, result Result) (Result, error) {
	if o.PreTagHook == "" {
		return result, nil
	}
	releaseVersion := ""
	if result.ReleaseVersion != nil {
		releaseVersion = result.ReleaseVersion.String()
	}
	env := map[string]string{
		"RELEASE_TAG":             result.ReleaseTag,
		"PREVIOUS_VERSION":        result.PreviousVersion,
		"PREVIOUS_REF":            result.PreviousRef,
		"PREVIOUS_STABLE_VERSION": result.PreviousStableVersion,
		"PREVIOUS_STABLE_REF":     result.PreviousStableRef,
		"FIRST_RELEASE":           strconv.FormatBool(result.FirstRelease),
		"GITHUB_TOKEN":            o.GithubToken,
		"RELEASE_NOTES_FILE":      o.releaseNotesFile(),
		"RELEASE_TARGET":          o.releaseTargetFile(),
		"ASSETS_DIR":              o.assetsDir(),
		"RELEASE_VERSION":         releaseVersion,
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
	_, err := o.runCmd(ctx, &runCmdOpts{
		stdout: stdout,
		stderr: stderr,
		env:    env,
	}, "sh", "-c", o.PreTagHook)
	result.PreTagHookOutput = stdoutBuf.String()
	result.PrereleaseHookOutput = stdoutBuf.String()
	if err != nil {
		const exitCodeAborted = 10
		exitErr := asExitErr(err)
		if exitErr != nil {
			if exitErr.ExitCode() == exitCodeAborted {
				slog.Debug("pre-tag hook aborted")
				result.PreTagHookAborted = true
				result.PrereleaseHookAborted = true
				return result, nil
			}
			err = exitErr
		}
		slog.Error(
			"pre-tag hook failed",
			slog.Any("err", err),
			slog.String("stdout", stdoutBuf.String()),
			slog.String("stderr", stderrBuf.String()),
		)
		return result, err
	}
	return result, nil
}

// isAllowedRef checks if the given commitish is one ot the allowed refs. Returns true when
// allowedRefs is empty.
func (o *Runner) isAllowedRef(ctx context.Context, commitish string, allowedRefs []string) bool {
	if len(allowedRefs) == 0 {
		return true
	}
	return o.gitNameRev(ctx, commitish, allowedRefs)
}

// gitNameRev checks if the given commitish (commit, branch, or tag) matches any of the provided refs
// using `git name-rev`. It returns true if the command succeeds, meaning the commitish can be resolved
// to one of the refs. This is useful for determining if a specific ref (e.g., a branch or tag) is present
// in a list of allowed refs, which helps control release or tagging logic based on repository state.
func (o *Runner) gitNameRev(ctx context.Context, commitish string, refs []string) bool {
	args := []string{"name-rev", commitish, "--no-undefined"}
	for _, ref := range refs {
		args = append(args, "--refs", ref)
	}
	_, err := o.runCmd(ctx, nil, "git", args...)
	return err == nil
}

// assertTagNotExists returns an error if tag exists either locally or on remote.
func (o *Runner) assertTagNotExists(ctx context.Context, remote, tag string) error {
	out, err := o.runCmd(ctx, nil, "git", "ls-remote", "--tags", remote, tag)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("tag %q already exists on remote", tag)
	}
	ok, err := localTagExists(ctx, o.CheckoutDir, tag)
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
