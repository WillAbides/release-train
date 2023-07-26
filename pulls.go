package main

import (
	"fmt"
	"sort"
)

type Pull struct {
	Number           int         `json:"number"`
	LevelLabels      []string    `json:"labels,omitempty"`
	ChangeLevel      ChangeLevel `json:"change_level"`
	HasPreLabel      bool        `json:"has_pre_label,omitempty"`
	PreReleasePrefix string      `json:"pre_release_prefix,omitempty"`
	HasStableLabel   bool        `json:"has_stable_label,omitempty"`
}

func NewPull(number int, aliases map[string]string, labels ...string) (*Pull, error) {
	p := Pull{
		Number:      number,
		ChangeLevel: ChangeLevelNone,
	}
	sort.Strings(labels)
	for _, label := range labels {
		resolvedLabel := ResolveLabel(label, aliases)
		level, ok := LabelLevels[resolvedLabel]
		if ok {
			p.LevelLabels = append(p.LevelLabels, label)
			if level > p.ChangeLevel {
				p.ChangeLevel = level
			}
		}
		pre, prefix := CheckPrereleaseLabel(label, nil)
		if pre {
			p.HasPreLabel = true
			if prefix != "" {
				if p.PreReleasePrefix != "" && p.PreReleasePrefix != prefix {
					return nil, fmt.Errorf("pull #%d has conflicting prerelease prefixes: %s and %s", number, p.PreReleasePrefix, prefix)
				}
				p.PreReleasePrefix = prefix
			}
		}
		if resolvedLabel == LabelStable {
			p.HasStableLabel = true
		}
	}
	if p.HasPreLabel && p.HasStableLabel {
		return nil, fmt.Errorf("pull #%d has both prerelease and stable labels", number)
	}
	return &p, nil
}

func (p Pull) String() string {
	return fmt.Sprintf("#%d", p.Number)
}
