package releasetrain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sethvargo/go-githubactions"
	"github.com/willabides/release-train-action/v3/internal"
	"github.com/willabides/release-train-action/v3/internal/labelcheck"
	"github.com/willabides/release-train-action/v3/internal/logging"
	"github.com/willabides/release-train-action/v3/internal/release"
	"golang.org/x/exp/slog"
	"gopkg.in/yaml.v3"
)

type rootCmd struct {
	Version        kong.VersionFlag  `action:"-"`
	GenerateAction bool              `hidden:"true" help:"${generate_action_help}"`
	Repo           string            `action:",${{ github.repository }}" help:"Github repository in the form of owner/repo."`
	CheckPR        int               `action:"check-pr,${{ github.event.number }}" help:"${check_pr_help}"`
	Label          map[string]string `action:"labels" help:"${label_help}" placeholder:"<alias>=<label>;..."`
	CheckoutDir    string            `action:",${{ github.workspace }}" short:"C" default:"." help:"${checkout_dir_help}"`
	Ref            string            `default:"HEAD" help:"${ref_help}"`
	GithubToken    string            `action:"github-token,${{ github.token }}" hidden:"true" env:"GITHUB_TOKEN" help:"${github_token_help}"`
	CreateTag      bool              `help:"${create_tag_help}"`
	CreateRelease  bool              `help:"${create_release_help}"`
	TagPrefix      string            `default:"v" help:"${tag_prefix_help}"`
	V0             bool              `name:"v0" help:"${v0_help}"`
	InitialTag     string            `action:"initial-release-tag" help:"${initial_tag_help}" default:"${initial_tag_default}"`
	PreReleaseHook string            `placeholder:"<command>" help:"${pre_release_hook_help}"`
	GoModFile      []string          `action:"validate-go-module" placeholder:"<filepath>" help:"${go_mod_file_help}"`
	ReleaseRef     []string          `action:"release-refs" placeholder:"<branch>" help:"${release_ref_help}"`
	PushRemote     string            `action:"-" default:"origin" help:"The git remote to push to."`
	Tempdir        string            `help:"The prefix to use with mktemp to create a temporary directory."`
	GithubApiUrl   string            `action:"-" help:"${github_api_url_help}" default:"${github_api_url_default}"`
	OutputFormat   string            `action:"-" default:"json" help:"${output_format_help}" enum:"json,action"`
	Debug          bool              `action:"-" help:"${debug_help}"`
}

func (c *rootCmd) GithubClient(ctx context.Context) (internal.GithubClient, error) {
	return internal.NewGithubClient(ctx, c.GithubApiUrl, c.GithubToken, fmt.Sprintf("release-train/%s", getVersion(ctx)))
}

func (c *rootCmd) Run(ctx context.Context, kongCtx *kong.Context) error {
	slogOpts := slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	if c.Debug {
		slogOpts.Level = slog.LevelDebug
	}
	var logHandler slog.Handler = slog.NewTextHandler(os.Stderr, &slogOpts)
	if c.OutputFormat == "action" {
		logHandler = logging.NewActionHandler(os.Stdout, &slogOpts)
	}
	ctx = logging.WithLogger(ctx, slog.New(logHandler))
	if c.GenerateAction {
		return c.generateAction(kongCtx)
	}
	if c.CheckPR != 0 {
		return c.runCheckPR(ctx)
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
	logger := logging.GetLogger(ctx)
	defer func() {
		if errOut != nil {
			logger.Error(errOut.Error())
		}
	}()
	logger.Info("starting runRelease")
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

func (c *rootCmd) runCheckPR(ctx context.Context) error {
	ghClient, err := c.GithubClient(ctx)
	if err != nil {
		return err
	}
	repo := c.Repo
	if repo == "" {
		repo, err = internal.GetGithubRepoFromRemote(c.CheckoutDir, c.PushRemote)
		if err != nil {
			return fmt.Errorf("could not determine github repo: %w", err)
		}
	}
	repoOwner, repoName, _ := strings.Cut(repo, "/")
	opts := labelcheck.Options{
		GhClient:     ghClient,
		PrNumber:     c.CheckPR,
		RepoOwner:    repoOwner,
		RepoName:     repoName,
		LabelAliases: c.Label,
	}
	return labelcheck.Check(ctx, &opts)
}
