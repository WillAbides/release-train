package releasetrain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sethvargo/go-githubactions"
	"github.com/willabides/release-train-action/v3/internal"
	"github.com/willabides/release-train-action/v3/internal/actions"
	"github.com/willabides/release-train-action/v3/internal/labelcheck"
	"github.com/willabides/release-train-action/v3/internal/release"
	"golang.org/x/exp/slog"
)

type actionContext struct {
	action   *actions.CompositeAction
	ghAction *githubactions.Action
	context  *githubactions.GitHubContext
	logger   *slog.Logger
}

func (a *actionContext) getInput(name string) string {
	_, ok := a.action.Inputs.Get(name)
	if !ok {
		panic(fmt.Sprintf("input %s not found", name))
	}
	name = strings.ReplaceAll(name, "-", "_")
	return a.ghAction.GetInput(name)
}

func (a *actionContext) getBoolInput(name string) bool {
	input := strings.TrimSpace(a.getInput(name))
	return input == "true"
}

func (a *actionContext) getSliceInput(name string) []string {
	input := strings.TrimSpace(a.getInput(name))
	if input == "" {
		return nil
	}
	var val []string
	input = strings.ReplaceAll(input, "\r\n", "\n")
	for _, v := range strings.Split(input, "\n") {
		if strings.TrimSpace(v) == "" {
			continue
		}
		val = append(val, v)
	}
	return val
}

func (a *actionContext) getMapInput(name string) (map[string]string, error) {
	sl := a.getSliceInput(name)
	if len(sl) == 0 {
		return nil, nil
	}
	val := make(map[string]string, len(sl))
	for _, line := range sl {
		if strings.Count(line, "=") != 1 {
			return nil, fmt.Errorf("invalid input for %s. each line must have exacly one '=': %q", name, line)
		}
		k, v, _ := strings.Cut(line, "=")
		_, exists := val[k]
		if exists {
			return nil, fmt.Errorf("duplicate key %q in input %s", k, name)
		}
		val[k] = v
	}
	return val, nil
}

func (cmd *actionContext) setOutput(name, value string) {
	_, ok := cmd.action.Outputs.Get(name)
	if !ok {
		panic(fmt.Sprintf("output %s not found", name))
	}
	cmd.logger.Debug("outputting", slog.String("name", name), slog.String("value", value))
	cmd.ghAction.SetOutput(name, value)
}

func runActionLabelCheck(ctx context.Context, actionCtx *actionContext) error {
	if actionCtx.getBoolInput(inputNoRelease) {
		actionCtx.logger.Info("skipping label check because no-release is true")
		return nil
	}
	eventPR, ok := actionCtx.context.Event["pull_request"].(map[string]any)
	if !ok {
		return fmt.Errorf("event is not a pull request")
	}
	prNumberFloat, ok := eventPR["number"].(float64)
	if !ok {
		return fmt.Errorf("event pull request has no number")
	}
	prNumber := int(prNumberFloat)
	ghClient, err := internal.NewGithubClient(ctx, actionCtx.context.APIURL, actionCtx.getInput(inputGithubToken), fmt.Sprintf("release-train/%s", getVersion(ctx)))
	if err != nil {
		return err
	}
	repoOwner, repoName := actionCtx.context.Repo()
	labelAliases, err := actionCtx.getMapInput(inputLabels)
	if err != nil {
		return err
	}
	opts := labelcheck.Options{
		GhClient:     ghClient,
		PrNumber:     prNumber,
		RepoOwner:    repoOwner,
		RepoName:     repoName,
		LabelAliases: labelAliases,
	}
	return labelcheck.Check(ctx, &opts)
}

func runActionRelease(ctx context.Context, cmd *actionContext) error {
	if cmd.getBoolInput(inputNoRelease) {
		cmd.logger.Info("skipping release because no-release is true")
		return nil
	}

	var err error

	ownerName, repoName := cmd.context.Repo()

	tmpDir := os.Getenv("RUNNER_TEMP")
	if tmpDir == "" {
		tmpDir, err = os.MkdirTemp("", "release-train-*")
		if err != nil {
			return err
		}
		defer func() {
			//nolint:errcheck // ignore error
			_ = os.RemoveAll(tmpDir)
		}()
	}

	ghClient, err := internal.NewGithubClient(ctx, cmd.context.APIURL, cmd.getInput(inputGithubToken), fmt.Sprintf("release-train/%s", getVersion(ctx)))
	if err != nil {
		return err
	}

	labelAliases, err := cmd.getMapInput(inputLabels)
	if err != nil {
		return err
	}

	runner := &release.Runner{
		CheckoutDir:    cmd.getInput(inputCheckoutDir),
		Ref:            cmd.getInput(inputRef),
		GithubToken:    cmd.getInput(inputGithubToken),
		CreateTag:      cmd.getBoolInput(inputCreateTag),
		CreateRelease:  cmd.getBoolInput(inputCreateRelease),
		TagPrefix:      cmd.getInput(inputTagPrefix),
		InitialTag:     cmd.getInput(inputInitialTag),
		PrereleaseHook: cmd.getInput(inputPreReleaseHook),
		GoModFiles:     cmd.getSliceInput(inputValidateGoMod),
		V0:             cmd.getBoolInput(inputV0),
		ReleaseRefs:    cmd.getSliceInput(inputReleaseRefs),
		PushRemote:     "origin",
		Repo:           fmt.Sprintf("%s/%s", ownerName, repoName),
		TempDir:        tmpDir,
		LabelAliases:   labelAliases,

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
