// Package action is just a struct to represent a github action.yml file.
package action

import (
	"fmt"

	"gopkg.in/yaml.v3"
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
	Name             string              `yaml:"name,omitempty"`
	Id               string              `yaml:"id,omitempty"`
	If               string              `yaml:"if,omitempty"`
	Shell            string              `yaml:"shell,omitempty"`
	WorkingDirectory string              `yaml:"working-directory,omitempty"`
	Env              *OrderedMap[string] `yaml:"env,omitempty"`
	Run              string              `yaml:"run,omitempty"`
	Uses             string              `yaml:"uses,omitempty"`
	With             *OrderedMap[string] `yaml:"with,omitempty"`
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
	Name        string                       `yaml:"name"`
	Description string                       `yaml:"description"`
	Author      string                       `yaml:"author,omitempty"`
	Branding    *Branding                    `yaml:"branding,omitempty"`
	Inputs      *OrderedMap[Input]           `yaml:"inputs,omitempty"`
	Outputs     *OrderedMap[CompositeOutput] `yaml:"outputs,omitempty"`
	Runs        CompositeRuns                `yaml:"runs"`
}

type OrderedMapTuple[V any] struct {
	Key   string
	Value V
}

func MapEntry[V any](key string, value V) OrderedMapTuple[V] {
	return OrderedMapTuple[V]{Key: key, Value: value}
}

func NewOrderedMap[V any](tuples ...OrderedMapTuple[V]) *OrderedMap[V] {
	m := &OrderedMap[V]{}
	for _, tuple := range tuples {
		m.Add(tuple.Key, tuple.Value)
	}
	return m
}

type OrderedMap[V any] struct {
	keys   []string
	values map[string]V
}

func (m *OrderedMap[V]) Add(key string, value V) {
	if m.values == nil {
		m.values = map[string]V{}
	}
	m.values[key] = value
	m.keys = append(m.keys, key)
}

func (m *OrderedMap[V]) Get(key string) V {
	if m.values == nil {
		var zero V
		return zero
	}
	return m.values[key]
}

func (m *OrderedMap[V]) MarshalYAML() (any, error) {
	var node yaml.Node
	node.Kind = yaml.MappingNode
	node.Tag = "!!map"
	node.Content = make([]*yaml.Node, 0, len(m.keys)*2)
	for _, key := range m.keys {
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: key,
		})

		valNode := &yaml.Node{}
		err := valNode.Encode(m.values[key])
		if err != nil {
			return nil, err
		}

		node.Content = append(node.Content, valNode)
	}
	return node, nil
}

func (m *OrderedMap[V]) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node, got %v", node.Kind)
	}
	m.values = map[string]V{}
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		var key string
		err := keyNode.Decode(&key)
		if err != nil {
			return err
		}
		var val V
		err = valNode.Decode(&val)
		if err != nil {
			return err
		}
		m.values[key] = val
		m.keys = append(m.keys, key)
	}
	return nil
}
