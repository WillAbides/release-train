package main

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/alecthomas/kong"
	orderedmap "github.com/wk8/go-ordered-map/v2"
)

const (
	actionBoolSuffix  = "Only literal 'true' will be treated as true."
	actionSliceSuffix = "Accepts multiple values. One value per line."
)

var outputItems = []struct {
	name        string
	description string
	value       func(*Result) string
}{
	{
		name:        "previous-ref",
		description: `A git ref pointing to the previous release, or the current ref if no previous release can be found.`,
		value:       func(r *Result) string { return r.PreviousRef },
	},
	{
		name:        "previous-version",
		description: `The previous version on the release branch.`,
		value:       func(r *Result) string { return r.PreviousVersion },
	},
	{
		name:        "first-release",
		description: `Whether this is the first release on the release branch. Either "true" or "false".`,
		value:       func(r *Result) string { return fmt.Sprintf("%t", r.FirstRelease) },
	},
	{
		name:        "release-version",
		description: `The version of the new release. Empty if no release is called for.`,
		value:       func(r *Result) string { return r.ReleaseVersion.String() },
	},
	{
		name:        "release-tag",
		description: `The tag of the new release. Empty if no release is called for.`,
		value:       func(r *Result) string { return r.ReleaseTag },
	},
	{
		name:        "change-level",
		description: `The level of change in the release. Either "major", "minor", "patch" or "none".`,
		value:       func(r *Result) string { return r.ChangeLevel.String() },
	},
	{
		name:        "created-tag",
		description: `Whether a tag was created. Either "true" or "false".`,
		value:       func(r *Result) string { return fmt.Sprintf("%t", r.CreatedTag) },
	},
	{
		name:        "created-release",
		description: `Whether a release was created. Either "true" or "false".`,
		value:       func(r *Result) string { return fmt.Sprintf("%t", r.CreatedRelease) },
	},
	{
		name:        "pre-release-hook-output",
		description: `The stdout of the pre_release_hook. Empty if pre_release_hook is not set or if the hook returned an exit other than 0 or 10.`,
		value:       func(r *Result) string { return r.PrereleaseHookOutput },
	},
	{
		name:        "pre-release-hook-aborted",
		description: `Whether pre_release_hook issued an abort by exiting 10. Either "true" or "false".`,
		value:       func(r *Result) string { return fmt.Sprintf("%t", r.PrereleaseHookAborted) },
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

func getAction(kongCtx *kong.Context) (*CompositeAction, error) {
	script := `#!/bin/sh
set -e

ACTION_DIR="${{ github.action_path }}"
RELEASE_TRAIN_BIN="$ACTION_DIR"/bin/release-train

if [ -z "${{ inputs.release-train-bin }}" ]; then
  RELEASE_TRAIN_BIN="$ACTION_DIR"/script/release-train
else
  RELEASE_TRAIN_BIN="${{ inputs.release-train-bin }}"
fi

set -- --output-format action --debug
`
	inputs := orderedmap.New[string, Input]()

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
		inputs.Set(actionInput, Input{
			Description: actionHelp,
			Default:     actionDefault,
		})

		script += buf.String()
	}
	script += `
"$RELEASE_TRAIN_BIN" "$@"
`
	inputs.AddPairs(
		orderedmap.Pair[string, Input]{
			Key: "release-train-bin",
			Value: Input{
				Description: "Path to release-train binary. Only needed if you're using a custom release-train binary.",
			},
		},
	)

	releaseOutput := func(s string) string {
		return fmt.Sprintf("${{ steps.release.outputs.%s }}", s)
	}

	outputs := orderedmap.New[string, CompositeOutput]()
	for _, item := range outputItems {
		outputs.Set(item.name, CompositeOutput{
			Value:       releaseOutput(item.name),
			Description: item.description,
		})
	}

	action := CompositeAction{
		Name:        kongCtx.Model.Name,
		Description: kongCtx.Model.Help,
		Branding: &Branding{
			Icon:  "send",
			Color: "yellow",
		},
		Inputs:  inputs,
		Outputs: outputs,
		Runs: CompositeRuns{
			Using: "composite",
			Steps: []CompositeStep{
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

type Input struct {
	DeprecationMessage string `yaml:"deprecationMessage,omitempty"`
	Description        string `yaml:"description"`
	Required           bool   `yaml:"required,omitempty"`
	Default            string `yaml:"default,omitempty"`
}

type CompositeOutput struct {
	Value       string `yaml:"value"`
	Description string `yaml:"description"`
}

type CompositeStep struct {
	Name             string                                 `yaml:"name,omitempty"`
	Id               string                                 `yaml:"id,omitempty"`
	If               string                                 `yaml:"if,omitempty"`
	Shell            string                                 `yaml:"shell,omitempty"`
	WorkingDirectory string                                 `yaml:"working-directory,omitempty"`
	Env              *orderedmap.OrderedMap[string, string] `yaml:"env,omitempty"`
	Run              string                                 `yaml:"run,omitempty"`
	Uses             string                                 `yaml:"uses,omitempty"`
	With             *orderedmap.OrderedMap[string, string] `yaml:"with,omitempty"`
}

type CompositeRuns struct {
	Using string          `yaml:"using"`
	Steps []CompositeStep `yaml:"steps"`
}

type Branding struct {
	Icon  string `yaml:"icon,omitempty"`
	Color string `yaml:"color,omitempty"`
}

type CompositeAction struct {
	Name        string                                          `yaml:"name"`
	Description string                                          `yaml:"description"`
	Author      string                                          `yaml:"author,omitempty"`
	Branding    *Branding                                       `yaml:"branding,omitempty"`
	Inputs      *orderedmap.OrderedMap[string, Input]           `yaml:"inputs,omitempty"`
	Outputs     *orderedmap.OrderedMap[string, CompositeOutput] `yaml:"outputs,omitempty"`
	Runs        CompositeRuns                                   `yaml:"runs"`
}
