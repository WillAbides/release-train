package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/sethvargo/go-githubactions"
	"github.com/willabides/actionslog"
	"github.com/willabides/actionslog/human"
	"golang.org/x/exp/slog"
	"gopkg.in/yaml.v3"
)

var version = "dev"

var flagHelp = kong.Vars{
	"generate_action_help": `Ignore all other flags and generate a GitHub action.`,
	"ref_help":             `git ref.`,
	"checkout_dir_help":    `The directory where the repository is checked out.`,
	"create_tag_help":      `Whether to create a tag for the release.`,
	"create_release_help":  `Whether to create a release. Implies create-tag.`,
	"initial_tag_help":     `The tag to use if no previous version can be found. Set to "" to cause an error instead.`,
	"tag_prefix_help":      `The prefix to use for the tag.`,
	"label_help":           `PR label alias in the form of "<alias>=<label>" where <label> is a canonical label.`,
	"output_format_help":   `Output either json our GitHub action output.`,
	"debug_help":           `Enable debug logging.`,
	"draft_help":           `Leave the release as a draft.`,
	"tempdir_help":         `The prefix to use with mktemp to create a temporary directory.`,
	"pushremote_help":      `The remote to push tags to.`,
	"repo_help":            `Github repository in the form of owner/repo.`,
	"github_token_help":    "The GitHub token to use for authentication. Must have `contents: write` permission if creating a release or tag.",
	"github_api_url_help":  `GitHub API URL.`,

	"check_pr_help": `
Operates as if the given PR has already been merged. Useful for making sure the PR is properly labeled.
Skips tag and release.
`,

	"pre_release_hook_help": `
Command to run before creating the release. You may abort the release by exiting with a non-zero exit code.
  
Exit code 0 will continue the release. Exit code 10 will skip the release without error. Any other exit code will
abort the release with an error.

You may provide custom release notes by writing to the file at $RELEASE_NOTES_FILE:

    echo "my release notes" > "$RELEASE_NOTES_FILE"

You can update the git ref to be released by writing it to the file at $RELEASE_TARGET:

    # ... update some files ...
    git commit -am "prepare release $RELEASE_TAG"
    echo "$(git rev-parse HEAD)" > "$RELEASE_TARGET"

If you create a tag named $RELEASE_TAG, it will be used as the release target instead of either HEAD or the value
written to $RELEASE_TARGET.

Any files written to $ASSETS_DIR will be uploaded as release assets.

The environment variables RELEASE_VERSION, RELEASE_TAG, PREVIOUS_VERSION, FIRST_RELEASE, GITHUB_TOKEN,
RELEASE_NOTES_FILE, RELEASE_TARGET and ASSETS_DIR will be set.
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
	Version        kong.VersionFlag  `action:"-"`
	GenerateAction bool              `hidden:"true" help:"${generate_action_help}"`
	Repo           string            `action:",${{ github.repository }}" help:"${repo_help}"`
	CheckPR        int               `action:"check-pr,${{ github.event.number }}" help:"${check_pr_help}"`
	Label          map[string]string `action:"labels" help:"${label_help}" placeholder:"<alias>=<label>;..."`
	CheckoutDir    string            `action:",${{ github.workspace }}" short:"C" default:"." help:"${checkout_dir_help}"`
	Ref            string            `default:"HEAD" help:"${ref_help}"`
	GithubToken    string            `action:"github-token,${{ github.token }}" hidden:"true" env:"GITHUB_TOKEN" help:"${github_token_help}"`
	CreateTag      bool              `help:"${create_tag_help}"`
	CreateRelease  bool              `help:"${create_release_help}"`
	Draft          bool              `help:"${draft_help}"`
	TagPrefix      string            `default:"v" help:"${tag_prefix_help}"`
	V0             bool              `name:"v0" help:"${v0_help}"`
	InitialTag     string            `action:"initial-release-tag" help:"${initial_tag_help}" default:"v0.0.0"`
	PreReleaseHook string            `placeholder:"<command>" help:"${pre_release_hook_help}"`
	ReleaseRef     []string          `action:"release-refs" placeholder:"<branch>" help:"${release_ref_help}"`
	PushRemote     string            `action:"-" default:"origin" help:"${pushremote_help}"`
	Tempdir        string            `help:"${tempdir_help}"`
	GithubApiUrl   string            `action:"-" help:"${github_api_url_help}" default:"https://api.github.com"`
	OutputFormat   string            `action:"-" default:"json" help:"${output_format_help}" enum:"json,action"`
	Debug          bool              `help:"${debug_help}"`
}

func (c *rootCmd) GithubClient(ctx context.Context) (GithubClient, error) {
	return NewGithubClient(ctx, c.GithubApiUrl, c.GithubToken, fmt.Sprintf("release-train/%s", version))
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
				if l == actionslog.LogDebug {
					l = actionslog.LogNotice
				}
				return l
			},
			Handler: (&human.Handler{
				Level: slog.LevelDebug,
			}).WithOutput,
		}
	}
	ctx = withLogger(ctx, slog.New(logHandler))
	if c.GenerateAction {
		return c.generateAction(kongCtx)
	}
	return c.runRelease(ctx)
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

func (c *rootCmd) runRelease(ctx context.Context) (errOut error) {
	logger := getLogger(ctx)
	defer func() {
		if errOut != nil {
			logger.Error(errOut.Error())
		}
	}()
	logger.Debug("starting runRelease")
	ghClient, err := c.GithubClient(ctx)
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
		repo, err = getGithubRepoFromRemote(c.CheckoutDir, c.PushRemote)
		if err != nil {
			return err
		}
	}

	runner := &Runner{
		CheckoutDir:    c.CheckoutDir,
		LabelAliases:   c.Label,
		Ref:            c.Ref,
		GithubToken:    c.GithubToken,
		CreateTag:      createTag,
		CreateRelease:  c.CreateRelease,
		Draft:          c.Draft,
		TagPrefix:      c.TagPrefix,
		InitialTag:     c.InitialTag,
		PrereleaseHook: c.PreReleaseHook,
		PushRemote:     c.PushRemote,
		Repo:           repo,
		TempDir:        tempDir,
		V0:             c.V0,
		ReleaseRefs:    c.ReleaseRef,
		CheckPR:        c.CheckPR,

		GithubClient: ghClient,
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
