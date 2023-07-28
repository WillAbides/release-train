package main

import (
	"fmt"
	"sort"
)

type ghPull struct {
	Number           int         `json:"number"`
	LevelLabels      []string    `json:"labels,omitempty"`
	ChangeLevel      changeLevel `json:"change_level"`
	HasPreLabel      bool        `json:"has_pre_label,omitempty"`
	PreReleasePrefix string      `json:"pre_release_prefix,omitempty"`
	HasStableLabel   bool        `json:"has_stable_label,omitempty"`
}

func newPull(number int, aliases map[string]string, labels ...string) (*ghPull, error) {
	p := ghPull{
		Number:      number,
		ChangeLevel: changeLevelNone,
	}
	sort.Strings(labels)
	for _, label := range labels {
		resolvedLabel := ResolveLabel(label, aliases)
		level, ok := labelLevels[resolvedLabel]
		if ok {
			p.LevelLabels = append(p.LevelLabels, label)
			if level > p.ChangeLevel {
				p.ChangeLevel = level
			}
		}
		pre, prefix := checkPrereleaseLabel(label, nil)
		if pre {
			p.HasPreLabel = true
			if prefix != "" {
				if p.PreReleasePrefix != "" && p.PreReleasePrefix != prefix {
					return nil, fmt.Errorf("pull #%d has conflicting prerelease prefixes: %s and %s", number, p.PreReleasePrefix, prefix)
				}
				p.PreReleasePrefix = prefix
			}
		}
		if resolvedLabel == labelStable {
			p.HasStableLabel = true
		}
	}
	if p.HasPreLabel && p.HasStableLabel {
		return nil, fmt.Errorf("pull #%d has both prerelease and stable labels", number)
	}
	return &p, nil
}

func (p ghPull) String() string {
	return fmt.Sprintf("#%d", p.Number)
}
