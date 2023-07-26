package actions

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	orderedmap "github.com/wk8/go-ordered-map/v2"
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

		Outputs: orderedmap.New[string, CompositeOutput](
			orderedmap.WithInitialData[string, CompositeOutput](
				orderedmap.Pair[string, CompositeOutput]{
					Key: "test",
					Value: CompositeOutput{
						Value:       "test",
						Description: "this is a test",
					},
				},
			),
		),
		Inputs: orderedmap.New[string, Input](
			orderedmap.WithInitialData[string, Input](
				orderedmap.Pair[string, Input]{
					Key: "test",
					Value: Input{
						DeprecationMessage: "omg this is deprecated",
						Description:        "we are testing\nthis",
						Required:           true,
						Default:            "${{ github.event.inputs.test }}",
					},
				},
			),
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
					Env: orderedmap.New[string, string](
						orderedmap.WithInitialData[string, string](
							orderedmap.Pair[string, string]{
								Key:   "test",
								Value: "test",
							},
						),
					),
					Run:  "test",
					Uses: "test",
					With: orderedmap.New[string, string](
						orderedmap.WithInitialData[string, string](
							orderedmap.Pair[string, string]{
								Key:   "test",
								Value: "test",
							},
						),
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
