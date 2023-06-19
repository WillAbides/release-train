package releasetrain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/sethvargo/go-githubactions"
	"github.com/willabides/release-train-action/v3/internal/action"
	"github.com/willabides/release-train-action/v3/internal/actionlogger"
	"github.com/willabides/release-train-action/v3/internal/labelcheck"
	"github.com/willabides/release-train-action/v3/internal/orderedmap"
	"github.com/willabides/release-train-action/v3/internal/release"
	"golang.org/x/exp/slog"
	"gopkg.in/yaml.v3"
)

const (
	actionBoolSuffix = "Only literal 'true' will be treated as true."
	actionCsvSuffix  = "Comma separated list of values without spaces."
)

const (
	inputCheckPRLabels  = "check-pr-labels"
	inputCheckoutDir    = "checkout-dir"
	inputRef            = "ref"
	inputGithubToken    = "github-token"
	inputCreateTag      = "create-tag"
	inputCreateRelease  = "create-release"
	inputTagPrefix      = "tag-prefix"
	inputV0             = "v0"
	inputInitialTag     = "initial-release-tag"
	inputPreReleaseHook = "pre-release-hook"
	inputValidateGoMod  = "validate-go-module"
	inputReleaseRefs    = "release-refs"
	inputNoRelease      = "no-release"
)

const (
	outputPreviousRef           = "previous-ref"
	outputPreviousVersion       = "previous-version"
	outputFirstRelease          = "first-release"
	outputReleaseVersion        = "release-version"
	outputReleaseTag            = "release-tag"
	outputChangeLevel           = "change-level"
	outputCreatedTag            = "created-tag"
	outputCreatedRelease        = "created-release"
	outputPreReleaseHookOutput  = "pre-release-hook-output"
	outputPreReleaseHookAborted = "pre-release-hook-aborted"
)

type actionCmd struct {
	Output *actionOutputCmd `kong:"cmd,hidden,help='Output the action yaml to stdout'"`
	Run    *actionRunCmd    `kong:"cmd,help='Run as a GitHub action.'"`
}

type actionOutputCmd struct{}

func (cmd *actionOutputCmd) Run(kongCtx *kong.Context) error {
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	return enc.Encode(getAction(kongCtx))
}

type actionRunCmd struct {
	action   *action.CompositeAction
	ghAction *githubactions.Action
	context  *githubactions.GitHubContext
	logger   *slog.Logger
}

func (cmd *actionRunCmd) BeforeApply(kongCtx *kong.Context) error {
	var err error
	cmd.action = getAction(kongCtx)
	cmd.ghAction = githubactions.New()
	cmd.logger = slog.New(actionlogger.NewHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: false,
	}))
	cmd.context, err = cmd.ghAction.Context()
	return err
}

func (cmd *actionRunCmd) getInput(name string) string {
	_, ok := cmd.action.Inputs.Get(name)
	if !ok {
		panic(fmt.Sprintf("input %s not found", name))
	}
	name = strings.ReplaceAll(name, "-", "_")
	return cmd.ghAction.GetInput(name)
}

func (cmd *actionRunCmd) setOutput(name, value string) {
	_, ok := cmd.action.Outputs.Get(name)
	if !ok {
		panic(fmt.Sprintf("output %s not found", name))
	}
	cmd.logger.Debug("outputting", slog.String("name", name), slog.String("value", value))
	cmd.ghAction.SetOutput(name, value)
}

func (cmd *actionRunCmd) Run(ctx context.Context) (errOut error) {
	defer func() {
		if errOut != nil {
			cmd.logger.Error(errOut.Error())
		}
	}()
	if cmd.getInput(inputCheckPRLabels) == "true" {
		return cmd.runLabelCheck(ctx)
	}
	return cmd.runRelease(ctx)
}

func (cmd *actionRunCmd) runLabelCheck(ctx context.Context) error {
	if cmd.getInput(inputNoRelease) == "true" {
		cmd.logger.Info("skipping label check because no-release is true")
		return nil
	}
	eventPR, ok := cmd.context.Event["pull_request"].(map[string]any)
	if !ok {
		return fmt.Errorf("event is not a pull request")
	}
	prNumberFloat, ok := eventPR["number"].(float64)
	if !ok {
		return fmt.Errorf("event pull request has no number")
	}
	prNumber := int(prNumberFloat)
	ghClientConfig := &githubClientConfig{
		GithubToken:  cmd.getInput(inputGithubToken),
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
	if cmd.getInput(inputNoRelease) == "true" {
		cmd.logger.Info("skipping release because no-release is true")
		return nil
	}

	var goModFiles []string
	if cmd.getInput(inputValidateGoMod) != "" {
		goModFiles = []string{cmd.getInput(inputValidateGoMod)}
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
	if cmd.getInput(inputReleaseRefs) != "" {
		releaseRefs = strings.Split(cmd.getInput(inputReleaseRefs), ",")
	}

	ghClientConfig := &githubClientConfig{
		GithubToken:  cmd.getInput(inputGithubToken),
		GithubApiUrl: cmd.context.APIURL,
	}
	ghClient, err := ghClientConfig.Client(ctx)
	if err != nil {
		return err
	}

	runner := &release.Runner{
		CheckoutDir:    cmd.getInput(inputCheckoutDir),
		Ref:            cmd.getInput(inputRef),
		GithubToken:    cmd.getInput(inputGithubToken),
		CreateTag:      cmd.getInput(inputCreateTag) == "true",
		CreateRelease:  cmd.getInput(inputCreateRelease) == "true",
		TagPrefix:      cmd.getInput(inputTagPrefix),
		InitialTag:     cmd.getInput(inputInitialTag),
		PrereleaseHook: cmd.getInput(inputPreReleaseHook),
		GoModFiles:     goModFiles,
		PushRemote:     "origin",
		Repo:           fmt.Sprintf("%s/%s", ownerName, repoName),
		TempDir:        tmpDir,
		V0:             cmd.getInput(inputV0) == "true",
		ReleaseRefs:    releaseRefs,

		GithubClient: ghClient,
	}

	b, err := json.Marshal(runner)
	if err != nil {
		return err
	}
	cmd.logger.Debug("running", slog.String("runner", string(b)))

	result, err := runner.Run(ctx)
	if err != nil {
		return err
	}

	b, err = json.Marshal(result)
	if err != nil {
		return err
	}
	cmd.logger.Debug("got result", slog.String("result", string(b)))

	cmd.setOutput(outputPreviousRef, result.PreviousRef)
	cmd.setOutput(outputPreviousVersion, result.PreviousVersion)
	cmd.setOutput(outputFirstRelease, fmt.Sprintf("%t", result.FirstRelease))
	cmd.setOutput(outputReleaseVersion, result.ReleaseVersion.String())
	cmd.setOutput(outputReleaseTag, result.ReleaseTag)
	cmd.setOutput(outputChangeLevel, result.ChangeLevel.String())
	cmd.setOutput(outputCreatedTag, fmt.Sprintf("%t", result.CreatedTag))
	cmd.setOutput(outputCreatedRelease, fmt.Sprintf("%t", result.CreatedRelease))
	cmd.setOutput(outputPreReleaseHookOutput, result.PrereleaseHookOutput)
	cmd.setOutput(outputPreReleaseHookAborted, fmt.Sprintf("%t", result.PrereleaseHookAborted))

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
	inputs := orderedmap.NewOrderedMap(
		orderedmap.Pair(inputCheckPRLabels, action.Input{
			Description: `Instead of releasing, check that the PR has a label indicating the type of change.` +
				"\n\n" + actionBoolSuffix,
			Default: "${{ github.event_name == 'pull_request' }}",
		}),

		orderedmap.Pair(inputCheckoutDir, action.Input{
			Description: getVar("checkout_dir_help"),
			Default:     "${{ github.workspace }}",
		}),

		orderedmap.Pair(inputRef, action.Input{
			Description: getVar("ref_help"),
			Default:     "${{ github.ref }}",
		}),

		orderedmap.Pair(inputGithubToken, action.Input{
			Description: getVar("github_token_help"),
			Default:     "${{ github.token }}",
		}),

		orderedmap.Pair(inputCreateTag, action.Input{
			Description: getVar("create_tag_help") + "\n\n" + actionBoolSuffix,
		}),

		orderedmap.Pair(inputCreateRelease, action.Input{
			Description: getVar("create_release_help") + "\n\n" + actionBoolSuffix,
		}),

		orderedmap.Pair(inputTagPrefix, action.Input{
			Description: getVar("tag_prefix_help"),
			Default:     vars["tag_prefix_default"],
		}),

		orderedmap.Pair(inputV0, action.Input{
			Description: getVar("v0_help") + "\n\n" + actionBoolSuffix,
		}),

		orderedmap.Pair(inputInitialTag, action.Input{
			Description: getVar("initial_tag_help"),
			Default:     vars["initial_tag_default"],
		}),

		orderedmap.Pair(inputPreReleaseHook, action.Input{
			Description: getVar("pre_release_hook_help"),
		}),

		orderedmap.Pair(inputValidateGoMod, action.Input{
			Description: getVar("go_mod_file_help"),
		}),

		orderedmap.Pair(inputReleaseRefs, action.Input{
			Description: getVar("release_ref_help") + "\n\n" + actionCsvSuffix,
		}),

		orderedmap.Pair(inputNoRelease, action.Input{
			Description: `
If set to true, this will be a no-op. This is useful for creating a new repository or branch that isn't ready for
release yet.` + "\n\n" + actionBoolSuffix,
		}),
	)
	releaseStepEnv := orderedmap.NewOrderedMap(
		orderedmap.Pair("GITHUB_REPOSITORY", "${{ github.repository }}"),
	)
	for inputPair := inputs.Oldest(); inputPair != nil; inputPair = inputPair.Next() {
		envName := fmt.Sprintf("INPUT_%s", strings.ToUpper(inputPair.Key))
		envName = strings.ReplaceAll(envName, "-", "_")
		val := fmt.Sprintf("${{ inputs.%s }}", inputPair.Key)
		releaseStepEnv.AddPairs(orderedmap.Pair(envName, val))
	}

	releaseOutput := func(s string) string {
		return fmt.Sprintf("${{ steps.release.outputs.%s }}", s)
	}

	outputs := orderedmap.NewOrderedMap(
		orderedmap.Pair(outputPreviousRef, action.CompositeOutput{
			Value:       releaseOutput(outputPreviousRef),
			Description: "A git ref pointing to the previous release, or the current ref if no previous release can be found.",
		}),

		orderedmap.Pair(outputPreviousVersion, action.CompositeOutput{
			Value:       releaseOutput(outputPreviousVersion),
			Description: "The previous version on the release branch.",
		}),

		orderedmap.Pair(outputFirstRelease, action.CompositeOutput{
			Value:       releaseOutput(outputFirstRelease),
			Description: "Whether this is the first release on the release branch. Either \"true\" or \"false\".",
		}),

		orderedmap.Pair(outputReleaseVersion, action.CompositeOutput{
			Value:       releaseOutput(outputReleaseVersion),
			Description: "The version of the new release. Empty if no release is called for.",
		}),

		orderedmap.Pair(outputReleaseTag, action.CompositeOutput{
			Value:       releaseOutput(outputReleaseTag),
			Description: "The tag of the new release. Empty if no release is called for.",
		}),

		orderedmap.Pair(outputChangeLevel, action.CompositeOutput{
			Value:       releaseOutput(outputChangeLevel),
			Description: "The level of change in the release. Either \"major\", \"minor\", \"patch\" or \"no change\".",
		}),

		orderedmap.Pair(outputCreatedTag, action.CompositeOutput{
			Value:       releaseOutput(outputCreatedTag),
			Description: "Whether a tag was created. Either \"true\" or \"false\".",
		}),

		orderedmap.Pair(outputCreatedRelease, action.CompositeOutput{
			Value:       releaseOutput(outputCreatedRelease),
			Description: "Whether a release was created. Either \"true\" or \"false\".",
		}),

		orderedmap.Pair(outputPreReleaseHookOutput, action.CompositeOutput{
			Value:       releaseOutput(outputPreReleaseHookOutput),
			Description: "The stdout of the pre_release_hook. Empty if pre_release_hook is not set or if the hook returned an exit other than 0 or 10.",
		}),

		orderedmap.Pair(outputPreReleaseHookAborted, action.CompositeOutput{
			Value:       releaseOutput(outputPreReleaseHookAborted),
			Description: "Whether pre_release_hook issued an abort by exiting 10. Either \"true\" or \"false\".",
		}),
	)

	releaseStep := action.CompositeStep{
		Id:               "release",
		Shell:            "sh",
		WorkingDirectory: "${{ inputs.checkout_dir }}",
		Env:              releaseStepEnv,
		Run: `ACTION_DIR="${{ github.action_path }}"
if [ -z "$RELEASE_TRAIN_BIN" ]; then
  "$ACTION_DIR"/script/bindown -q install release-train --allow-missing-checksum
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
