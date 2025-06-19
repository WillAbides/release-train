package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/alecthomas/kong"
	"github.com/sethvargo/go-githubactions"
	"github.com/willabides/actionslog"
	"github.com/willabides/actionslog/human"
	"github.com/willabides/release-train/v3/internal/github"
	"gopkg.in/yaml.v3"
)

var version = "dev"

var flagHelp = kong.Vars{
	"generate_action_help":  `Ignore all other flags and generate a GitHub action.`,
	"ref_help":              `git ref.`,
	"checkout_dir_help":     `The directory where the repository is checked out.`,
	"create_tag_help":       `Whether to create a tag for the release.`,
	"create_release_help":   `Whether to create a release. Implies create-tag.`,
	"force_prerelease_help": `Force prerelease even if no prerelease PRs are present.`,
	"force_stable_help":     `Force stable release even if no stable PRs are present.`,
	"initial_tag_help":      `The tag to use if no previous version can be found. Set to "" to cause an error instead.`,
	"tag_prefix_help":       `The prefix to use for the tag.`,
	"label_help":            `PR label alias in the form of "<alias>=<label>" where <label> is a canonical label.`,
	"output_format_help":    `Output either json our GitHub action output.`,
	"debug_help":            `Enable debug logging.`,
	"draft_help":            `Leave the release as a draft.`,
	"tempdir_help":          `The prefix to use with mktemp to create a temporary directory.`,
	"pushremote_help":       `The remote to push tags to.`,
	"repo_help":             `GitHub repository in the form of owner/repo.`,
	"github_token_help":     "The GitHub token to use for authentication. Must have `contents: write` permission if creating a release or tag.",
	"github_api_url_help":   `GitHub API URL.`,

	"check_pr_help": `
Operates as if the given PR has already been merged. Useful for making sure the PR is properly labeled.
Skips tag and release.
`,
	"make_latest_help": `
Mark the release as "latest" on GitHub. Can be set to "true", "false" or "legacy". See 
https://docs.github.com/en/rest/releases/releases#update-a-release  for details.`,

	"pre_tag_hook_help": `
Command to run before tagging the release. You may abort the release by exiting with a non-zero exit code. Exit code 0
will continue the release. Exit code 10 will skip the release without error. Any other exit code will abort the release
with an error.

Environment variables available to the hook:

    RELEASE_VERSION
      The semantic version being released (e.g. 1.2.3).

    RELEASE_TAG
      The tag being created (e.g. v1.2.3).

    PREVIOUS_VERSION 
      The previous semantic version (e.g. 1.2.2). Empty on
      first release.

    PREVIOUS_REF
      The git ref of the previous release (e.g. v1.2.2). Empty on
      first release.

    PREVIOUS_STABLE_VERSION
      The previous stable semantic version (e.g. 1.2.2). Empty if there
      hasn't been a stable version yet. A stable version is one without
      prerelease identifiers.

    PREVIOUS_STABLE_REF
      The git ref of the previous stable release (e.g. v1.2.2). Empty if there
      hasn't been a stable version yet. A stable version is one without
      prerelease identifiers.

    FIRST_RELEASE
      Whether this is the first release. Either "true" or
      "false".

    GITHUB_TOKEN
      The GitHub token that was provided to release-train.

    RELEASE_NOTES_FILE
      A file path where you can write custom release notes.
      When nothing is written to this file, release-train
      will use GitHub's default release notes.

    RELEASE_TARGET
      A file path where you can write an alternate git ref
      to release instead of HEAD.

    ASSETS_DIR
      A directory where you can write release assets. All
      files in this directory will be uploaded as release
      assets.

In addition to the above environment variables, all variables from release-train's environment are available to the
hook.

When the hook creates a tag named $RELEASE_TAG, it will be used as the release target instead of either HEAD or the
value written to $RELEASE_TARGET.
`,

	"pre_release_hook_help": `
*deprecated* Will be removed in a future release. Alias for pre-tag-hook.
`,

	"v0_help": `
Assert that current major version is 0 and treat breaking changes as minor changes.
Errors if the major version is not 0.
`,

	"release_ref_help": `
Only allow tags and releases to be created from matching refs. Refs can be patterns accepted by git-show-ref.
If undefined, any branch can be used.
`,
}

func main() {
	ctx := context.Background()
	vars := flagHelp
	vars["version"] = version

	var root rootCmd
	k := kong.Parse(
		&root,
		vars,
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.Description("Release every PR merge. No magic commit message required."),
	)
	k.FatalIfErrorf(k.Run())
}

type rootCmd struct {
	Version         kong.VersionFlag  `action:"-"`
	GenerateAction  bool              `hidden:"true" help:"${generate_action_help}"`
	Repo            string            `action:",${{ github.repository }}" help:"${repo_help}"`
	CheckPR         int               `action:"check-pr,${{ github.event.number }}" help:"${check_pr_help}"`
	Label           map[string]string `action:"labels" help:"${label_help}" placeholder:"<alias>=<label>;..."`
	CheckoutDir     string            `action:",${{ github.workspace }}" short:"C" default:"." help:"${checkout_dir_help}"`
	Ref             string            `default:"HEAD" help:"${ref_help}"`
	GithubToken     string            `action:"github-token,${{ github.token }}" hidden:"true" env:"GITHUB_TOKEN" help:"${github_token_help}"`
	CreateTag       bool              `help:"${create_tag_help}"`
	CreateRelease   bool              `help:"${create_release_help}"`
	ForcePrerelease bool              `help:"${force_prerelease_help}"`
	ForceStable     bool              `help:"${force_stable_help}"`
	Draft           bool              `help:"${draft_help}"`
	TagPrefix       string            `default:"v" help:"${tag_prefix_help}"`
	V0              bool              `name:"v0" help:"${v0_help}"`
	InitialTag      string            `action:"initial-release-tag" help:"${initial_tag_help}" default:"v0.0.0"`
	MakeLatest      string            `action:"make-latest" default:"legacy" help:"${make_latest_help}" enum:"legacy,true,false"`
	PreTagHook      string            `placeholder:"<command>" help:"${pre_tag_hook_help}"`
	PreReleaseHook  string            `placeholder:"<command>" help:"${pre_release_hook_help}"`
	ReleaseRef      []string          `action:"release-refs" placeholder:"<branch>" help:"${release_ref_help}"`
	PushRemote      string            `action:"-" default:"origin" help:"${pushremote_help}"`
	Tempdir         string            `help:"${tempdir_help}"`
	GithubApiUrl    string            `action:"-" help:"${github_api_url_help}" default:"https://api.github.com"`
	OutputFormat    string            `action:"-" default:"json" help:"${output_format_help}" enum:"json,action"`
	Debug           bool              `help:"${debug_help}"`
}

func (c *rootCmd) GithubClient() (GithubClient, error) {
	return github.NewClient(c.GithubApiUrl, c.GithubToken, fmt.Sprintf("release-train/%s", version))
}

func (c *rootCmd) Run(ctx context.Context, kongCtx *kong.Context) error {
	var slogOpts slog.HandlerOptions
	if c.Debug {
		slogOpts.Level = slog.LevelDebug
	}
	var logHandler slog.Handler = slog.NewTextHandler(os.Stderr, &slogOpts)

	// In actions, we always log at debug level, but when --debug is set, we output the debug logs as notices.
	if c.OutputFormat == "action" {
		logHandler = &actionslog.Wrapper{
			ActionsLogger: func(level slog.Level) actionslog.ActionsLog {
				l := actionslog.DefaultActionsLog(level)
				if c.Debug && l == actionslog.LogDebug {
					l = actionslog.LogNotice
				}
				return l
			},
			Handler: (&human.Handler{
				Level: slog.LevelDebug,
			}).WithOutput,
		}
	}
	slog.SetDefault(slog.New(logHandler))
	if c.GenerateAction {
		return c.generateAction(kongCtx)
	}
	return c.runRelease(ctx, kongCtx.Stdout, kongCtx.Stderr)
}

func (c *rootCmd) generateAction(kongCtx *kong.Context) error {
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	got, err := getAction(kongCtx)
	if err != nil {
		return err
	}
	return enc.Encode(got)
}

func (c *rootCmd) runRelease(ctx context.Context, stdout, stderr io.Writer) (errOut error) {
	slog.Debug("starting runRelease")
	client, err := c.GithubClient()
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(c.Tempdir, "release-train-*")
	if err != nil {
		return err
	}
	defer func() {
		errOut = errors.Join(errOut, os.RemoveAll(tempDir))
	}()
	createTag := c.CreateTag
	if c.CreateRelease {
		createTag = true
	}

	repo := c.Repo
	if repo == "" {
		repo, err = getGithubRepoFromRemote(ctx, c.CheckoutDir, c.PushRemote)
		if err != nil {
			return err
		}
	}

	preTagHook := c.PreTagHook
	if c.PreReleaseHook != "" {
		if preTagHook != "" {
			return errors.New("cannot specify both --pre-tag-hook and --pre-release-hook")
		}
		preTagHook = c.PreReleaseHook
	}

	runner := &Runner{
		CheckoutDir:     c.CheckoutDir,
		Ref:             c.Ref,
		GithubToken:     c.GithubToken,
		CreateTag:       createTag,
		CreateRelease:   c.CreateRelease,
		Draft:           c.Draft,
		V0:              c.V0,
		TagPrefix:       c.TagPrefix,
		InitialTag:      c.InitialTag,
		PreTagHook:      preTagHook,
		Repo:            repo,
		PushRemote:      c.PushRemote,
		TempDir:         tempDir,
		ReleaseRefs:     c.ReleaseRef,
		LabelAliases:    c.Label,
		CheckPR:         c.CheckPR,
		GithubClient:    client,
		Stdout:          stdout,
		Stderr:          stderr,
		ForcePrerelease: c.ForcePrerelease,
		ForceStable:     c.ForceStable,
		MakeLatest:      c.MakeLatest,
	}

	result, err := runner.Run(ctx)
	if err != nil {
		return err
	}

	if c.OutputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	action := githubactions.New()
	for _, item := range outputItems {
		action.SetOutput(item.name, item.value(result))
	}
	return nil
}
