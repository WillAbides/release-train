package releasetrain

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/alecthomas/kong"
	"github.com/willabides/release-train-action/v3/internal/actions"
	"github.com/willabides/release-train-action/v3/internal/orderedmap"
)

const (
	actionBoolSuffix  = "Only literal 'true' will be treated as true."
	actionSliceSuffix = "Accepts multiple values. One value per line."
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

type actionInputHelper struct {
	Name string
	Flag string
}

func (a *actionInputHelper) Input() string {
	return fmt.Sprintf("${{ inputs.%s }}", a.Name)
}

var boolTemplate = template.Must(template.New("").Parse(`
case "{{.Input}}" in
  true)
    set -- "$@" --{{.Flag}}
    ;;
  false) ;;
  "") ;;
  *)
    echo "Input {{.Name}} must be 'true' or 'false'. Got '{{.Input}}'." >&2
    exit 1
	;;
esac
`))

var cumulativeTemplate = template.Must(template.New("").Parse(`
while IFS= read -r line; do
  [ -n "$line" ] || continue
  set -- "$@" --{{.Flag}} "$line"
done <<EOF
{{.Input}}
EOF
`))

var scalarTemplate = template.Must(template.New("").Parse(`
if [ -n "{{.Input}}" ]; then
  set -- "$@" --{{.Flag}} "{{.Input}}"
fi
`))

func getAction(kongCtx *kong.Context) (*actions.CompositeAction, error) {
	script := `#!/bin/sh
set -e

ACTION_DIR="${{ github.action_path }}"
RELEASE_TRAIN_BIN="$ACTION_DIR"/bin/release-train

if [ -n "${{ inputs.release-train-bin }}" ]; then
  RELEASE_TRAIN_BIN="${{ inputs.release-train-bin }}"
fi

set --
`
	inputs := orderedmap.NewOrderedMap(
		orderedmap.Pair("release-train-bin", actions.Input{
			Description: "Path to release-train binary. Only needed if you're using a custom release-train binary.",
		}),
	)

	for _, flag := range kongCtx.Flags() {
		if flag.Name == "help" {
			continue
		}
		tag := flag.Tag
		if flag.Hidden && !tag.Has("action") {
			continue
		}
		actionInput := flag.Name
		actionDefault := flag.Default
		if tag.Has("action") {
			input, def, hasDef := strings.Cut(tag.Get("action"), ",")
			if input == "-" {
				continue
			}
			if input != "" {
				actionInput = input
			}
			if hasDef {
				actionDefault = def
			}
		}
		actionHelp := strings.TrimSpace(flag.Help)
		if actionHelp == "" {
			return nil, fmt.Errorf("flag %s has no help text", flag.Name)
		}
		tmpl := scalarTemplate
		switch {
		case flag.IsBool():
			actionHelp += "\n\n" + actionBoolSuffix
			tmpl = boolTemplate
		case flag.IsCumulative():
			actionHelp += "\n\n" + actionSliceSuffix
			tmpl = cumulativeTemplate
		}
		var buf bytes.Buffer
		err := tmpl.Execute(&buf, &actionInputHelper{
			Name: actionInput,
			Flag: flag.Name,
		})
		if err != nil {
			return nil, err
		}
		inputs.Set(actionInput, actions.Input{
			Description: actionHelp,
			Default:     actionDefault,
		})

		script += buf.String()
	}
	script += `
"$RELEASE_TRAIN_BIN" "$@"
`

	releaseOutput := func(s string) string {
		return fmt.Sprintf("${{ steps.release.outputs.%s }}", s)
	}

	outputs := orderedmap.NewOrderedMap(
		orderedmap.Pair(outputPreviousRef, actions.CompositeOutput{
			Value:       releaseOutput(outputPreviousRef),
			Description: "A git ref pointing to the previous release, or the current ref if no previous release can be found.",
		}),

		orderedmap.Pair(outputPreviousVersion, actions.CompositeOutput{
			Value:       releaseOutput(outputPreviousVersion),
			Description: "The previous version on the release branch.",
		}),

		orderedmap.Pair(outputFirstRelease, actions.CompositeOutput{
			Value:       releaseOutput(outputFirstRelease),
			Description: "Whether this is the first release on the release branch. Either \"true\" or \"false\".",
		}),

		orderedmap.Pair(outputReleaseVersion, actions.CompositeOutput{
			Value:       releaseOutput(outputReleaseVersion),
			Description: "The version of the new release. Empty if no release is called for.",
		}),

		orderedmap.Pair(outputReleaseTag, actions.CompositeOutput{
			Value:       releaseOutput(outputReleaseTag),
			Description: "The tag of the new release. Empty if no release is called for.",
		}),

		orderedmap.Pair(outputChangeLevel, actions.CompositeOutput{
			Value:       releaseOutput(outputChangeLevel),
			Description: "The level of change in the release. Either \"major\", \"minor\", \"patch\" or \"none\".",
		}),

		orderedmap.Pair(outputCreatedTag, actions.CompositeOutput{
			Value:       releaseOutput(outputCreatedTag),
			Description: "Whether a tag was created. Either \"true\" or \"false\".",
		}),

		orderedmap.Pair(outputCreatedRelease, actions.CompositeOutput{
			Value:       releaseOutput(outputCreatedRelease),
			Description: "Whether a release was created. Either \"true\" or \"false\".",
		}),

		orderedmap.Pair(outputPreReleaseHookOutput, actions.CompositeOutput{
			Value:       releaseOutput(outputPreReleaseHookOutput),
			Description: "The stdout of the pre_release_hook. Empty if pre_release_hook is not set or if the hook returned an exit other than 0 or 10.",
		}),

		orderedmap.Pair(outputPreReleaseHookAborted, actions.CompositeOutput{
			Value:       releaseOutput(outputPreReleaseHookAborted),
			Description: "Whether pre_release_hook issued an abort by exiting 10. Either \"true\" or \"false\".",
		}),
	)

	action := actions.CompositeAction{
		Name:        kongCtx.Model.Name,
		Description: kongCtx.Model.Help,
		Branding: &actions.Branding{
			Icon:  "send",
			Color: "yellow",
		},
		Inputs:  inputs,
		Outputs: outputs,
		Runs: actions.CompositeRuns{
			Using: "composite",
			Steps: []actions.CompositeStep{
				{
					Name:  "release",
					Id:    "release",
					Shell: "sh",
					Run:   script,
				},
			},
		},
	}
	return &action, nil
}
