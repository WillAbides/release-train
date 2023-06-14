package releasetrain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/sethvargo/go-githubactions"
	"github.com/willabides/release-train-action/v2/internal/release"
)

type releaseCmd struct {
	Repo               string   `kong:"arg,help='Github repository in the form of owner/repo.'"`
	Ref                string   `kong:"default=HEAD,help=${ref_help}"`
	CreateTag          bool     `kong:"help=${create_tag_help}"`
	CreateRelease      bool     `kong:"help=${create_release_help}"`
	GoModFile          []string `kong:"placeholder=<filepath>,help=${go_mod_file_help}"`
	InitialTag         string   `kong:"help=${initial_tag_help},default=${initial_tag_default}"`
	PreReleaseHook     string   `kong:"placeholder=<command>,help=${pre_release_hook_help}"`
	PostReleaseHook    string   `kong:"placeholder=<command>"`
	TagPrefix          string   `kong:"default=v,help=${tag_prefix_help}"`
	V0                 bool     `kong:"name=v0,help=${v0_help}"`
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

	runner := &release.Runner{
		CheckoutDir:     root.CheckoutDir,
		Ref:             cmd.Ref,
		GithubToken:     cmd.GithubToken,
		CreateTag:       createTag,
		CreateRelease:   cmd.CreateRelease,
		TagPrefix:       cmd.TagPrefix,
		InitialTag:      cmd.InitialTag,
		PrereleaseHook:  cmd.PreReleaseHook,
		PostreleaseHook: cmd.PostReleaseHook,
		GoModFiles:      goModFiles,
		PushRemote:      cmd.PushRemote,
		Repo:            cmd.Repo,
		TempDir:         tempDir,

		GithubClient: ghClient,
	}

	result, err := runner.Run(ctx)
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
