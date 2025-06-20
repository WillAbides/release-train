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
	actionTagName        = "action"
	deprecatedMarker     = "*deprecated*"
	actionBoolSuffix     = "Only literal 'true' will be treated as true."
	actionSliceSuffix    = "Accepts multiple values. One value per line."
	compositeRunner      = "composite"
	defaultShell         = "sh"
	releaseTrainBinInput = "release-train-bin"
	releaseTrainBinHelp  = "Path to release-train binary. Only needed if you're using a custom release-train binary."
	releaseStepID        = "release"
)

type actionInputHelper struct {
	Name string
	Flag string
}

func (a *actionInputHelper) Input() string {
	return fmt.Sprintf("${{ inputs.%s }}", a.Name)
}

type actionInput struct {
	DeprecationMessage string `yaml:"deprecationMessage,omitempty"`
	Description        string `yaml:"description"`
	Required           bool   `yaml:"required,omitempty"`
	Default            string `yaml:"default,omitempty"`
}

type compositeOutput struct {
	Value       string `yaml:"value"`
	Description string `yaml:"description"`
}

type compositeStep struct {
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

type compositeRuns struct {
	Using string          `yaml:"using"`
	Steps []compositeStep `yaml:"steps"`
}

type branding struct {
	Icon  string `yaml:"icon,omitempty"`
	Color string `yaml:"color,omitempty"`
}

type compositeAction struct {
	Name        string                                          `yaml:"name"`
	Description string                                          `yaml:"description"`
	Author      string                                          `yaml:"author,omitempty"`
	Branding    *branding                                       `yaml:"branding,omitempty"`
	Inputs      *orderedmap.OrderedMap[string, actionInput]     `yaml:"inputs,omitempty"`
	Outputs     *orderedmap.OrderedMap[string, compositeOutput] `yaml:"outputs,omitempty"`
	Runs        compositeRuns                                   `yaml:"runs"`
}

// actionBuilder encapsulates the logic and state for building a compositeAction.
type actionBuilder struct {
	kongCtx        *kong.Context
	boolTmpl       *template.Template
	cumulativeTmpl *template.Template
	scalarTmpl     *template.Template
}

const boolTemplateStr = `
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
`

const cumulativeTemplateStr = `
while IFS= read -r line; do
  [ -n "$line" ] || continue
  set -- "$@" --{{.Flag}} "$line"
done <<EOF
{{.Input}}
EOF
`

const scalarTemplateStr = `
if [ -n "{{.Input}}" ]; then
  set -- "$@" --{{.Flag}} '{{.Input}}'
fi
`

// newActionBuilder creates and initializes a new actionBuilder.
func newActionBuilder(kongCtx *kong.Context) *actionBuilder {
	return &actionBuilder{
		kongCtx:        kongCtx,
		boolTmpl:       template.Must(template.New("bool").Parse(boolTemplateStr)),
		cumulativeTmpl: template.Must(template.New("cumulative").Parse(cumulativeTemplateStr)),
		scalarTmpl:     template.Must(template.New("scalar").Parse(scalarTemplateStr)),
	}
}

// Build is the main method that constructs the final compositeAction.
func (b *actionBuilder) Build() (*compositeAction, error) {
	inputs, runScript, err := b.buildInputsAndScript()
	if err != nil {
		return nil, fmt.Errorf("could not build action inputs and script: %w", err)
	}

	outputs := b.buildOutputs()

	action := &compositeAction{
		Name:        b.kongCtx.Model.Name,
		Description: b.kongCtx.Model.Help,
		Branding: &branding{
			Icon:  "send",
			Color: "yellow",
		},
		Inputs:  inputs,
		Outputs: outputs,
		Runs: compositeRuns{
			Using: compositeRunner,
			Steps: []compositeStep{
				{
					Name:  releaseStepID,
					Id:    releaseStepID,
					Shell: defaultShell,
					Run:   runScript,
				},
			},
		},
	}
	return action, nil
}

// buildInputsAndScript iterates over flags to create the complete inputs map and script.
func (b *actionBuilder) buildInputsAndScript() (*orderedmap.OrderedMap[string, actionInput], string, error) {
	var scriptBuilder strings.Builder
	scriptBuilder.WriteString(`#!/bin/sh
set -e

ACTION_DIR="${{ github.action_path }}"

RELEASE_TRAIN_BIN="$ACTION_DIR"/script/release-train
if [ -n "${{ inputs.release-train-bin }}" ]; then
  RELEASE_TRAIN_BIN="${{ inputs.release-train-bin }}"
fi

set -- --output-format action
`)
	inputs := orderedmap.New[string, actionInput]()

	for _, flag := range b.kongCtx.Flags() {
		name, input, scriptSnippet, err := b.processFlag(flag)
		if err != nil {
			return nil, "", err // Propagate error
		}
		if input != nil {
			inputs.Set(name, *input)
			scriptBuilder.WriteString(scriptSnippet)
		}
	}

	scriptBuilder.WriteString(`
"$RELEASE_TRAIN_BIN" "$@"
`)

	inputs.Set(releaseTrainBinInput, actionInput{
		Description: releaseTrainBinHelp,
	})

	return inputs, scriptBuilder.String(), nil
}

// processFlag handles the logic for a single flag, returning its input definition and script snippet.
func (b *actionBuilder) processFlag(flag *kong.Flag) (string, *actionInput, string, error) {
	tag := flag.Tag
	if (flag.Hidden && !tag.Has(actionTagName)) || flag.Name == "help" {
		return "", nil, "", nil // Skip this flag
	}

	actionInputName := flag.Name
	actionDefault := flag.Default
	if tag.Has(actionTagName) {
		input, def, hasDef := strings.Cut(tag.Get(actionTagName), ",")
		if input == "-" {
			return "", nil, "", nil // Explicitly skipped
		}
		if input != "" {
			actionInputName = input
		}
		if hasDef {
			actionDefault = def
		}
	}

	actionHelp := strings.TrimSpace(flag.Help)
	if actionHelp == "" {
		return "", nil, "", fmt.Errorf("flag %q has no help text", flag.Name)
	}

	var selectedTmpl *template.Template
	switch {
	case flag.IsBool():
		actionHelp += "\n\n" + actionBoolSuffix
		selectedTmpl = b.boolTmpl
	case flag.IsCumulative():
		actionHelp += "\n\n" + actionSliceSuffix
		selectedTmpl = b.cumulativeTmpl
	default:
		selectedTmpl = b.scalarTmpl
	}

	var tplBuffer bytes.Buffer
	err := selectedTmpl.Execute(&tplBuffer, &actionInputHelper{
		Name: actionInputName,
		Flag: flag.Name,
	})
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to execute template for flag %q: %w", flag.Name, err)
	}

	deprecationMessage := ""
	if strings.Contains(strings.ToLower(actionHelp), deprecatedMarker) {
		deprecationMessage = "deprecated"
	}
	input := &actionInput{
		Description:        actionHelp,
		Default:            actionDefault,
		DeprecationMessage: deprecationMessage,
	}

	return actionInputName, input, tplBuffer.String(), nil
}

// buildOutputs creates the map of action outputs.
func (b *actionBuilder) buildOutputs() *orderedmap.OrderedMap[string, compositeOutput] {
	outputs := orderedmap.New[string, compositeOutput]()
	releaseOutputValue := func(s string) string {
		return fmt.Sprintf("${{ steps.%s.outputs.%s }}", releaseStepID, s)
	}
	for _, item := range outputItems() {
		outputs.Set(item.name, compositeOutput{
			Value:       releaseOutputValue(item.name),
			Description: item.description,
		})
	}
	return outputs
}

// getAction is the public entry point that creates and runs the actionBuilder.
func getAction(kongCtx *kong.Context) (*compositeAction, error) {
	return newActionBuilder(kongCtx).Build()
}
