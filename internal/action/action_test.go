package action

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train-action/v3/internal/orderedmap"
	"gopkg.in/yaml.v3"
)

func TestAction(t *testing.T) {
	action := CompositeAction{
		Name:        "my action",
		Description: "this is\na test",
		Author:      "me",
		Branding: &Branding{
			Icon:  "test",
			Color: "test",
		},
		Inputs: orderedmap.NewOrderedMap(
			orderedmap.Pair("test", Input{
				DeprecationMessage: "omg this is deprecated",
				Description:        "we are testing\nthis",
				Required:           true,
				Default:            "${{ github.event.inputs.test }}",
			}),
		),
		Outputs: orderedmap.NewOrderedMap(
			orderedmap.Pair("test", CompositeOutput{
				Value:       "test",
				Description: "this is a test",
			}),
		),
		Runs: CompositeRuns{
			Using: "composite",
			Steps: []CompositeStep{
				{
					Name:             "test",
					Id:               "test",
					If:               "test",
					Shell:            "test",
					WorkingDirectory: "test",
					Env: orderedmap.NewOrderedMap(
						orderedmap.Pair("test", "test"),
					),
					Run:  "test",
					Uses: "test",
					With: orderedmap.NewOrderedMap(
						orderedmap.Pair("test", "test"),
					),
				},
			},
		},
	}
	want := `
name: my action
description: |-
  this is
  a test
author: me
branding:
  icon: test
  color: test
inputs:
  test:
    deprecationMessage: omg this is deprecated
    description: |-
      we are testing
      this
    required: true
    default: ${{ github.event.inputs.test }}
outputs:
  test:
    value: test
    description: this is a test
runs:
  using: composite
  steps:
    - name: test
      id: test
      if: test
      shell: test
      working-directory: test
      env:
        test: test
      run: test
      uses: test
      with:
        test: test
`
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(&action)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(want), strings.TrimSpace(buf.String()))
	var action2 CompositeAction
	err = yaml.Unmarshal(buf.Bytes(), &action2)
	require.NoError(t, err)
	require.Equal(t, action, action2)
}
