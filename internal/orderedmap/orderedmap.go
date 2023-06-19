package orderedmap

import (
	"fmt"

	orderedmap "github.com/wk8/go-ordered-map/v2"
	"gopkg.in/yaml.v3"
)

func Pair[V any](key string, value V) orderedmap.Pair[string, V] {
	return orderedmap.Pair[string, V]{Key: key, Value: value}
}

func NewOrderedMap[V any](pairs ...orderedmap.Pair[string, V]) *OrderedMap[V] {
	return &OrderedMap[V]{
		OrderedMap: orderedmap.New[string, V](orderedmap.WithInitialData(pairs...)),
	}
}

type OrderedMap[V any] struct {
	*orderedmap.OrderedMap[string, V]
}

func (m *OrderedMap[V]) MarshalYAML() (any, error) {
	var node yaml.Node
	node.Kind = yaml.MappingNode
	node.Tag = "!!map"
	for pair := m.Oldest(); pair != nil; pair = pair.Next() {
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: pair.Key,
		})
		valNode := &yaml.Node{}
		err := valNode.Encode(pair.Value)
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

	m.OrderedMap = orderedmap.New[string, V]()
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
		m.OrderedMap.AddPairs(Pair(key, val))
	}
	return nil
}
