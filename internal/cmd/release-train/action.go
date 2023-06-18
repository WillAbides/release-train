package releasetrain

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sethvargo/go-githubactions"
	"github.com/willabides/release-train-action/v3/internal/action"
	"github.com/willabides/release-train-action/v3/internal/labelcheck"
	"github.com/willabides/release-train-action/v3/internal/release"
	"gopkg.in/yaml.v3"
)

const (
	actionBoolSuffix = "Only literal 'true' will be treated as true."
	actionCsvSuffix  = "Comma separated list of values without spaces."
)

type actionCmd struct {
	Output *actionOutputCmd `kong:"cmd,hidden,help='Output the action yaml to stdout'"`
	Run    *actionRunCmd    `kong:"cmd,help='Run as a GitHub action.'"`
}

type actionOutputCmd struct{}

func (cmd *actionOutputCmd) Run(kongCtx *kong.Context) error {
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	kongCtx.Model.Vars()
	return enc.Encode(getAction(kongCtx))
}

type actionRunCmd struct {
	action   *action.CompositeAction
	ghAction *githubactions.Action
	context  *githubactions.GitHubContext
}

func (cmd *actionRunCmd) BeforeApply(kongCtx *kong.Context) error {
	var err error
	cmd.action = getAction(kongCtx)
	cmd.ghAction = githubactions.New()
	cmd.context, err = cmd.ghAction.Context()
	return err
}

func (cmd *actionRunCmd) getInput(name string) string {
	if !cmd.action.Inputs.Has(name) {
		panic(fmt.Sprintf("input %s not found", name))
	}
	return cmd.ghAction.GetInput(name)
}

func (cmd *actionRunCmd) setOutput(name, value string) {
	if !cmd.action.Outputs.Has(name) {
		panic(fmt.Sprintf("output %s not found", name))
	}
	cmd.ghAction.SetOutput(name, value)
}

func (cmd *actionRunCmd) Run(ctx context.Context) (errOut error) {
	defer func() {
		if errOut != nil {
			cmd.ghAction.Errorf("%s", errOut)
		}
	}()
	if cmd.getInput("check_pr_labels") == "true" {
		return cmd.runLabelCheck(ctx)
	}
	return cmd.runRelease(ctx)
}

func (cmd *actionRunCmd) runLabelCheck(ctx context.Context) error {
	if cmd.getInput("no_release") == "true" {
		cmd.ghAction.Infof("Skipping check")
		return nil
	}
	eventPR, ok := cmd.context.Event["pull_request"].(map[string]any)
	if !ok {
		return fmt.Errorf("event is not a pull request")
	}
	prNumber, ok := eventPR["number"].(int)
	if !ok {
		return fmt.Errorf("event pull request has no number")
	}
	ghClientConfig := &githubClientConfig{
		GithubToken:  cmd.getInput("github_token"),
		GithubApiUrl: cmd.context.APIURL,
	}
	ghClient, err := ghClientConfig.Client(ctx)
	if err != nil {
		return err
	}
	repoOwner, repoName := cmd.context.Repo()
	opts := labelcheck.Options{
		GhClient:  ghClient,
		PrNumber:  prNumber,
		RepoOwner: repoOwner,
		RepoName:  repoName,
	}
	return labelcheck.Check(ctx, &opts)
}

func (cmd *actionRunCmd) runRelease(ctx context.Context) error {
	if cmd.getInput("no_release") == "true" {
		cmd.ghAction.Infof("Skipping release creation")
		return nil
	}

	var goModFiles []string
	if cmd.getInput("validate_go_module") != "" {
		goModFiles = []string{cmd.getInput("validate_go_module")}
	}

	var err error

	ownerName, repoName := cmd.context.Repo()

	tmpDir := os.Getenv("RUNNER_TEMP")
	if tmpDir == "" {
		tmpDir, err = os.MkdirTemp("", "release-train-*")
		if err != nil {
			return err
		}
	}

	var releaseRefs []string
	if cmd.getInput("release_refs") != "" {
		releaseRefs = strings.Split(cmd.getInput("release_refs"), ",")
	}

	ghClientConfig := &githubClientConfig{
		GithubToken:  cmd.getInput("github_token"),
		GithubApiUrl: cmd.context.APIURL,
	}
	ghClient, err := ghClientConfig.Client(ctx)
	if err != nil {
		return err
	}

	runner := &release.Runner{
		CheckoutDir:    cmd.getInput("checkout_dir"),
		Ref:            cmd.getInput("ref"),
		GithubToken:    cmd.getInput("github_token"),
		CreateTag:      cmd.getInput("create_tag") == "true",
		CreateRelease:  cmd.getInput("create_release") == "true",
		TagPrefix:      cmd.getInput("tag_prefix"),
		InitialTag:     cmd.getInput("initial_release_tag"),
		PrereleaseHook: cmd.getInput("pre_release_hook"),
		GoModFiles:     goModFiles,
		PushRemote:     "origin",
		Repo:           fmt.Sprintf("%s/%s", ownerName, repoName),
		TempDir:        tmpDir,
		V0:             cmd.getInput("v0") == "true",
		ReleaseRefs:    releaseRefs,

		GithubClient: ghClient,
	}

	result, err := runner.Run(ctx)
	if err != nil {
		return err
	}

	cmd.setOutput("previous_ref", result.PreviousRef)
	cmd.setOutput("previous_version", result.PreviousVersion)
	cmd.setOutput("first_release", fmt.Sprintf("%t", result.FirstRelease))
	cmd.setOutput("release_version", result.ReleaseVersion.String())
	cmd.setOutput("release_tag", result.ReleaseTag)
	cmd.setOutput("change_level", result.ChangeLevel.String())
	cmd.setOutput("created_tag", fmt.Sprintf("%t", result.CreatedTag))
	cmd.setOutput("created_release", fmt.Sprintf("%t", result.CreatedRelease))
	cmd.setOutput("pre_release_hook_output", result.PrereleaseHookOutput)
	cmd.setOutput("pre_release_hook_aborted", fmt.Sprintf("%t", result.PrereleaseHookAborted))

	return nil
}

func getAction(kongCtx *kong.Context) *action.CompositeAction {
	vars := kongCtx.Model.Vars()
	getVar := func(name string) string {
		val, ok := vars[name]
		if !ok {
			panic(fmt.Sprintf("variable %s not found", name))
		}
		return val
	}
	inputs := action.NewOrderedMap(
		action.MapEntry("check_pr_labels", action.Input{
			Description: `Instead of releasing, check that the PR has a label indicating the type of change.` +
				"\n\n" + actionBoolSuffix,
			Default: "${{ github.event_name == 'pull_request' }}",
		}),

		action.MapEntry("checkout_dir", action.Input{
			Description: getVar("checkout_dir_help"),
			Default:     "${{ github.workspace }}",
		}),

		action.MapEntry("ref", action.Input{
			Description: getVar("ref_help"),
			Default:     "${{ github.ref }}",
		}),

		action.MapEntry("github_token", action.Input{
			Description: getVar("github_token_help"),
			Default:     "${{ github.token }}",
		}),

		action.MapEntry("create_tag", action.Input{
			Description: getVar("create_tag_help") + "\n\n" + actionBoolSuffix,
		}),

		action.MapEntry("create_release", action.Input{
			Description: getVar("create_release_help") + "\n\n" + actionBoolSuffix,
		}),

		action.MapEntry("tag_prefix", action.Input{
			Description: getVar("tag_prefix_help"),
			Default:     vars["tag_prefix_default"],
		}),

		action.MapEntry("v0", action.Input{
			Description: getVar("v0_help") + "\n\n" + actionBoolSuffix,
		}),

		action.MapEntry("initial_release_tag", action.Input{
			Description: getVar("initial_tag_help"),
			Default:     vars["initial_tag_default"],
		}),

		action.MapEntry("pre_release_hook", action.Input{
			Description: getVar("pre_release_hook_help"),
		}),

		action.MapEntry("validate_go_module", action.Input{
			Description: getVar("go_mod_file_help"),
		}),

		action.MapEntry("release_refs", action.Input{
			Description: getVar("release_ref_help") + "\n\n" + actionCsvSuffix,
		}),

		action.MapEntry("no_release", action.Input{
			Description: `
If set to true, this will be a no-op. This is useful for creating a new repository or branch that isn't ready for
release yet.` + "\n\n" + actionBoolSuffix,
		}),
	)
	releaseStepEnv := action.NewOrderedMap(
		action.MapEntry("GITHUB_REPOSITORY", "${{ github.repository }}"),
	)
	for _, inputName := range inputs.Keys() {
		envName := strings.ToUpper(inputName)
		envName = strings.ReplaceAll(envName, "-", "_")
		val := fmt.Sprintf("${{ inputs.%s }}", inputName)
		releaseStepEnv.Add(envName, val)
	}
	outputs := action.NewOrderedMap(
		action.MapEntry("previous_ref", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.previous_ref }}",
			Description: "A git ref pointing to the previous release, or the current ref if no previous release can be found.",
		}),

		action.MapEntry("previous_version", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.previous_version }}",
			Description: "The previous version on the release branch.",
		}),

		action.MapEntry("first_release", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.first_release }}",
			Description: "Whether this is the first release on the release branch. Either \"true\" or \"false\".",
		}),

		action.MapEntry("release_version", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.release_version }}",
			Description: "The version of the new release. Empty if no release is called for.",
		}),

		action.MapEntry("release_tag", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.release_tag }}",
			Description: "The tag of the new release. Empty if no release is called for.",
		}),

		action.MapEntry("change_level", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.change_level }}",
			Description: "The level of change in the release. Either \"major\", \"minor\", \"patch\" or \"no change\".",
		}),

		action.MapEntry("created_tag", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.created_tag }}",
			Description: "Whether a tag was created. Either \"true\" or \"false\".",
		}),

		action.MapEntry("created_release", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.created_release }}",
			Description: "Whether a release was created. Either \"true\" or \"false\".",
		}),

		action.MapEntry("pre_release_hook_output", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.pre_release_hook_output }}",
			Description: "The stdout of the pre_release_hook. Empty if pre_release_hook is not set or if the hook returned an exit other than 0 or 10.",
		}),

		action.MapEntry("pre_release_hook_aborted", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.pre_release_hook_aborted }}",
			Description: "Whether pre_release_hook issued an abort by exiting 10. Either \"true\" or \"false\".",
		}),
	)

	releaseStep := action.CompositeStep{
		Id:               "release",
		Shell:            "sh",
		WorkingDirectory: "${{ inputs.checkout_dir }}",
		Run: `ACTION_DIR="${{ github.action_path }}"
if [ -z "$RELEASE_TRAIN_BIN" ]; then
  "$ACTION_DIR"/action/bindown -q install release-train --allow-missing-checksum
  RELEASE_TRAIN_BIN="$ACTION_DIR"/bin/release-train
fi

"$RELEASE_TRAIN_BIN" action run`,
	}

	return &action.CompositeAction{
		Name:        "release-train",
		Description: "release-train keeps a-rollin' down to San Antone",
		Branding: &action.Branding{
			Icon:  "send",
			Color: "yellow",
		},
		Inputs:  inputs,
		Outputs: outputs,
		Runs: action.CompositeRuns{
			Using: "composite",
			Steps: []action.CompositeStep{releaseStep},
		},
	}
}
