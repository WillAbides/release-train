package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v53/github"
	"github.com/sethvargo/go-githubactions"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

var version = "dev"

var helpVars = kong.Vars{
	"checkout_dir_help":     thisAction.Inputs.Get("checkout_dir").Description,
	"ref_help":              thisAction.Inputs.Get("ref").Description,
	"create_tag_help":       createTagHelp,
	"create_release_help":   createReleaseHelp,
	"go_mod_file_help":      validateGoModHelp,
	"initial_tag_help":      "The tag to use if no previous version can be found.",
	"pre_release_hook_help": thisAction.Inputs.Get("pre_release_hook").Description,
	"tag_prefix_help":       thisAction.Inputs.Get("tag_prefix").Description,
}

type rootCmd struct {
	CheckoutDir string     `kong:"short=C,default='.',help=${checkout_dir_help}"`
	Release     releaseCmd `kong:"cmd,help='Release a new version.'"`
	Action      actionCmd  `kong:"cmd,hidden,help='Create a composite action.'"`
	Version     kong.VersionFlag
}

type githubClientConfig struct {
	GithubToken  string `kong:"hidden,required,env=GITHUB_TOKEN,help='Credentials for Github API.'"`
	GithubApiUrl string `kong:"default=https://api.github.com,help='Github API URL.'"`
}

func (c *githubClientConfig) Client(ctx context.Context) (*github.Client, error) {
	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: c.GithubToken},
	))
	rateLimitClient, err := github_ratelimit.NewRateLimitWaiterClient(oauthClient.Transport)
	if err != nil {
		return nil, err
	}
	// no need for uploadURL because if we upload release artifacts we will use release.UploadURL
	ghClient, err := github.NewEnterpriseClient(c.GithubApiUrl, "", rateLimitClient)
	if err != nil {
		return nil, err
	}
	ghClient.UserAgent = fmt.Sprintf("release-train/%s", version)
	return ghClient, nil
}

type releaseCmd struct {
	Repo               string   `kong:"arg,help='Github repository in the form of owner/repo.'"`
	Ref                string   `kong:"default=HEAD,help=${ref_help}"`
	CreateTag          bool     `kong:"help=${create_tag_help}"`
	CreateRelease      bool     `kong:"help=${create_release_help}"`
	GoModFile          []string `kong:"placeholder=<filepath>,help=${go_mod_file_help}"`
	InitialTag         string   `kong:"placeholder=<tag>,help=${initial_tag_help}"`
	PreReleaseHook     string   `kong:"placeholder=<command>,help=${pre_release_hook_help}"`
	PostReleaseHook    string   `kong:"placeholder=<command>"`
	TagPrefix          string   `kong:"default=v,help=${tag_prefix_help}"`
	PushRemote         string   `kong:"default=origin,help='The git remote to push to.'"`
	Tempdir            string   `kong:"help='The prefix to use with mktemp to create a temporary directory.'"`
	githubClientConfig `kong:",embed"`
}

func (cmd *releaseCmd) Run(ctx context.Context, root *rootCmd) (errOut error) {
	ghClient, err := cmd.githubClientConfig.Client(ctx)
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(cmd.Tempdir, "release-train-*")
	if err != nil {
		return err
	}
	action := githubactions.New()
	actionCtx, err := action.Context()
	if err != nil {
		return err
	}
	defer func() {
		e := os.RemoveAll(tempDir)
		// It's normal to not be able to remove the tempdir in GitHub Actions when tempDir is in RUNNER_TEMP and
		// the action doesn't have the correct permissions.
		if actionCtx.Actions {
			return
		}
		errOut = errors.Join(errOut, e)
	}()

	createTag := cmd.CreateTag
	if cmd.CreateRelease {
		createTag = true
	}

	var goModFiles []string
	for _, goModFile := range cmd.GoModFile {
		if goModFile != "" {
			goModFiles = append(goModFiles, goModFile)
		}
	}

	runner := &releaseRunner{
		checkoutDir:     root.CheckoutDir,
		ref:             cmd.Ref,
		githubToken:     cmd.GithubToken,
		createTag:       createTag,
		createRelease:   cmd.CreateRelease,
		tagPrefix:       cmd.TagPrefix,
		initialTag:      cmd.InitialTag,
		prereleaseHook:  cmd.PreReleaseHook,
		postreleaseHook: cmd.PostReleaseHook,
		goModFiles:      goModFiles,
		pushRemote:      cmd.PushRemote,
		repo:            cmd.Repo,
		tempDir:         tempDir,

		githubClient: &ghWrapper{client: ghClient},
	}

	result, err := runner.run(ctx)
	if err != nil {
		return err
	}

	// Just output json if we aren't in an action.
	if !actionCtx.Actions {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	action.SetOutput("previous_ref", result.PreviousRef)
	action.SetOutput("previous_version", result.PreviousVersion)
	action.SetOutput("first_release", fmt.Sprintf("%t", result.FirstRelease))
	action.SetOutput("release_version", result.ReleaseVersion)
	action.SetOutput("release_tag", result.ReleaseTag)
	action.SetOutput("change_level", result.ChangeLevel.String())
	action.SetOutput("created_tag", fmt.Sprintf("%t", result.CreatedTag))
	action.SetOutput("created_release", fmt.Sprintf("%t", result.CreatedRelease))
	action.SetOutput("pre_release_hook_output", result.PrereleaseHookOutput)
	action.SetOutput("pre_release_hook_aborted", fmt.Sprintf("%t", result.PrereleaseHookAborted))

	return nil
}

type actionCmd struct{}

func (cmd *actionCmd) Run() error {
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	return enc.Encode(thisAction)
}

func main() {
	ctx := context.Background()
	var root rootCmd
	k := kong.Parse(
		&root,
		kong.Vars{
			"version": version,
		},
		helpVars,
		kong.BindTo(ctx, (*context.Context)(nil)),
	)
	k.FatalIfErrorf(k.Run(&root))
}
