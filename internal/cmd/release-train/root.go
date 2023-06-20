package releasetrain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/sethvargo/go-githubactions"
	"github.com/willabides/release-train-action/v3/internal"
	"github.com/willabides/release-train-action/v3/internal/actionlogger"
	"github.com/willabides/release-train-action/v3/internal/release"
	"golang.org/x/exp/slog"
	"gopkg.in/yaml.v3"
)

type rootCmd struct {
	Version        kong.VersionFlag
	Action         bool              `kong:"help=${action_help}"`
	GenerateAction bool              `kong:"hidden,help=${generate_action_help}"`
	CheckoutDir    string            `kong:"short=C,default='.',help=${checkout_dir_help}"`
	Label          map[string]string `kong:"help=${label_help}"`
	Repo           string            `kong:"help='Github repository in the form of owner/repo.'"`
	Ref            string            `kong:"default=HEAD,help=${ref_help}"`
	CreateTag      bool              `kong:"help=${create_tag_help}"`
	CreateRelease  bool              `kong:"help=${create_release_help}"`
	ReleaseRef     []string          `kong:"placeholder=<branch>,help=${release_ref_help}"`
	GoModFile      []string          `kong:"placeholder=<filepath>,help=${go_mod_file_help}"`
	InitialTag     string            `kong:"help=${initial_tag_help},default=${initial_tag_default}"`
	PreReleaseHook string            `kong:"placeholder=<command>,help=${pre_release_hook_help}"`
	TagPrefix      string            `kong:"default=v,help=${tag_prefix_help}"`
	V0             bool              `kong:"name=v0,help=${v0_help}"`
	PushRemote     string            `kong:"default=origin,help='The git remote to push to.'"`
	Tempdir        string            `kong:"help='The prefix to use with mktemp to create a temporary directory.'"`

	GithubToken  string `kong:"hidden,env=GITHUB_TOKEN,help=${github_token_help}"`
	GithubApiUrl string `kong:"help=${github_api_url_help},default=${github_api_url_default}"`
}

func (c *rootCmd) GithubClient(ctx context.Context) (internal.GithubClient, error) {
	return internal.NewGithubClient(ctx, c.GithubApiUrl, c.GithubToken, fmt.Sprintf("release-train/%s", getVersion(ctx)))
}

func (c *rootCmd) Run(ctx context.Context, kongCtx *kong.Context) error {
	if c.GenerateAction {
		return c.generateAction(kongCtx)
	}
	if c.Action {
		return c.runAction(ctx, kongCtx)
	}
	return c.runRelease(ctx)
}

func (c *rootCmd) generateAction(kongCtx *kong.Context) error {
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	return enc.Encode(getAction(kongCtx))
}

func (c *rootCmd) runAction(ctx context.Context, kongCtx *kong.Context) (errOut error) {
	actionCtx := &actionContext{
		action:   getAction(kongCtx),
		ghAction: githubactions.New(),
		logger: slog.New(actionlogger.NewHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
	}
	defer func() {
		if errOut != nil {
			actionCtx.logger.Error(errOut.Error())
		}
	}()
	ghContext, err := actionCtx.ghAction.Context()
	if err != nil {
		return err
	}
	actionCtx.context = ghContext

	if actionCtx.getBoolInput(inputCheckPRLabels) {
		return runActionLabelCheck(ctx, actionCtx)
	}
	return runActionRelease(ctx, actionCtx)
}

func (c *rootCmd) runRelease(ctx context.Context) (errOut error) {
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

	var goModFiles []string
	for _, goModFile := range c.GoModFile {
		if goModFile != "" {
			goModFiles = append(goModFiles, goModFile)
		}
	}

	runner := &release.Runner{
		CheckoutDir:    c.CheckoutDir,
		LabelAliases:   c.Label,
		Ref:            c.Ref,
		GithubToken:    c.GithubToken,
		CreateTag:      createTag,
		CreateRelease:  c.CreateRelease,
		TagPrefix:      c.TagPrefix,
		InitialTag:     c.InitialTag,
		PrereleaseHook: c.PreReleaseHook,
		GoModFiles:     goModFiles,
		PushRemote:     c.PushRemote,
		Repo:           c.Repo,
		TempDir:        tempDir,
		V0:             c.V0,
		ReleaseRefs:    c.ReleaseRef,

		GithubClient: ghClient,
	}

	result, err := runner.Run(ctx)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
