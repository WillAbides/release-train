// Package actions is just a struct to represent a github action.yml file.
package actions

import (
	"github.com/willabides/release-train-action/v3/internal/orderedmap"
)

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
	Name             string                         `yaml:"name,omitempty"`
	Id               string                         `yaml:"id,omitempty"`
	If               string                         `yaml:"if,omitempty"`
	Shell            string                         `yaml:"shell,omitempty"`
	WorkingDirectory string                         `yaml:"working-directory,omitempty"`
	Env              *orderedmap.OrderedMap[string] `yaml:"env,omitempty"`
	Run              string                         `yaml:"run,omitempty"`
	Uses             string                         `yaml:"uses,omitempty"`
	With             *orderedmap.OrderedMap[string] `yaml:"with,omitempty"`
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
	Name        string                                  `yaml:"name"`
	Description string                                  `yaml:"description"`
	Author      string                                  `yaml:"author,omitempty"`
	Branding    *Branding                               `yaml:"branding,omitempty"`
	Inputs      *orderedmap.OrderedMap[Input]           `yaml:"inputs,omitempty"`
	Outputs     *orderedmap.OrderedMap[CompositeOutput] `yaml:"outputs,omitempty"`
	Runs        CompositeRuns                           `yaml:"runs"`
}
