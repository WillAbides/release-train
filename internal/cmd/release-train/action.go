package releasetrain

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/alecthomas/kong"
	"github.com/willabides/release-train-action/v3/internal/actions"
	"github.com/willabides/release-train-action/v3/internal/orderedmap"
	"github.com/willabides/release-train-action/v3/internal/release"
)

const (
	actionBoolSuffix  = "Only literal 'true' will be treated as true."
	actionSliceSuffix = "Accepts multiple values. One value per line."
)

var outputItems = []struct {
	name        string
	description string
	value       func(*release.Result) string
}{
	{
		name:        "previous-ref",
		description: `A git ref pointing to the previous release, or the current ref if no previous release can be found.`,
		value:       func(r *release.Result) string { return r.PreviousRef },
	},
	{
		name:        "previous-version",
		description: `The previous version on the release branch.`,
		value:       func(r *release.Result) string { return r.PreviousVersion },
	},
	{
		name:        "first-release",
		description: `Whether this is the first release on the release branch. Either "true" or "false".`,
		value:       func(r *release.Result) string { return fmt.Sprintf("%t", r.FirstRelease) },
	},
	{
		name:        "release-version",
		description: `The version of the new release. Empty if no release is called for.`,
		value:       func(r *release.Result) string { return r.ReleaseVersion.String() },
	},
	{
		name:        "release-tag",
		description: `The tag of the new release. Empty if no release is called for.`,
		value:       func(r *release.Result) string { return r.ReleaseTag },
	},
	{
		name:        "change-level",
		description: `The level of change in the release. Either "major", "minor", "patch" or "none".`,
		value:       func(r *release.Result) string { return r.ChangeLevel.String() },
	},
	{
		name:        "created-tag",
		description: `Whether a tag was created. Either "true" or "false".`,
		value:       func(r *release.Result) string { return fmt.Sprintf("%t", r.CreatedTag) },
	},
	{
		name:        "created-release",
		description: `Whether a release was created. Either "true" or "false".`,
		value:       func(r *release.Result) string { return fmt.Sprintf("%t", r.CreatedRelease) },
	},
	{
		name:        "pre-release-hook-output",
		description: `The stdout of the pre_release_hook. Empty if pre_release_hook is not set or if the hook returned an exit other than 0 or 10.`,
		value:       func(r *release.Result) string { return r.PrereleaseHookOutput },
	},
	{
		name:        "pre-release-hook-aborted",
		description: `Whether pre_release_hook issued an abort by exiting 10. Either "true" or "false".`,
		value:       func(r *release.Result) string { return fmt.Sprintf("%t", r.PrereleaseHookAborted) },
	},
}

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
  set -- "$@" --{{.Flag}} '{{.Input}}'
fi
`))

func getAction(kongCtx *kong.Context) (*actions.CompositeAction, error) {
	script := `#!/bin/sh
set -ex

env

ACTION_DIR="${{ github.action_path }}"
RELEASE_TRAIN_BIN="$ACTION_DIR"/bin/release-train

if [ -z "${{ inputs.release-train-bin }}" ]; then
  RELEASE_TRAIN_BIN="$ACTION_DIR"/script/release-train
else
  RELEASE_TRAIN_BIN="${{ inputs.release-train-bin }}"
fi

set -- --output-format action --debug
`
	inputs := orderedmap.NewOrderedMap[actions.Input]()

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
	inputs.AddPairs(
		orderedmap.Pair("release-train-bin", actions.Input{
			Description: "Path to release-train binary. Only needed if you're using a custom release-train binary.",
		}),
	)

	releaseOutput := func(s string) string {
		return fmt.Sprintf("${{ steps.release.outputs.%s }}", s)
	}

	outputs := orderedmap.NewOrderedMap[actions.CompositeOutput]()
	for _, item := range outputItems {
		outputs.Set(item.name, actions.CompositeOutput{
			Value:       releaseOutput(item.name),
			Description: item.description,
		})
	}

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
