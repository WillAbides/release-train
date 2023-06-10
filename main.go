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
)

var version = "dev"

type rootCmd struct {
	CheckoutDir string     `kong:"short=C,default='.'"`
	Release     releaseCmd `kong:"cmd,help='Release a new version.'"`
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
	Ref                string   `kong:"default=HEAD"`
	CreateTag          bool     `kong:""`
	CreateRelease      bool     `kong:""`
	GoModFile          []string `kong:"placeholder=<filepath>"`
	InitialTag         string   `kong:"placeholder=<tag>"`
	PreReleaseHook     string   `kong:"placeholder=<command>"`
	PostReleaseHook    string   `kong:"placeholder=<command>"`
	TagPrefix          string   `kong:"default=v"`
	PushRemote         string   `kong:"default=origin"`
	Tempdir            string   `kong:""`
	githubClientConfig `kong:",embed"`
}

func (cmd *releaseCmd) Run(ctx context.Context, root *rootCmd) (errOut error) {
	ghClient, err := cmd.githubClientConfig.Client(ctx)
	if err != nil {
		return err
	}
	tmpdir, err := os.MkdirTemp(cmd.Tempdir, "release-train-*")
	if err != nil {
		return err
	}
	action := githubactions.New()
	actionCtx, err := action.Context()
	if err != nil {
		return err
	}
	defer func() {
		e := os.RemoveAll(tmpdir)
		// It's normal to not be able to remove the tempdir in GitHub Actions when tmpdir is in RUNNER_TEMP and
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

	return nil
}

func main() {
	ctx := context.Background()
	var root rootCmd
	k := kong.Parse(
		&root,
		kong.Vars{
			"version": version,
		},
		kong.BindTo(ctx, (*context.Context)(nil)),
	)
	k.FatalIfErrorf(k.Run(&root))
}
