package main

import (
	"fmt"
)

type gitCommit struct {
	Sha   string   `json:"sha"`
	Pulls []ghPull `json:"pulls,omitempty"`
}

func (c gitCommit) changeLevel() changeLevel {
	level := changeLevelNone
	for _, pull := range c.Pulls {
		if pull.ChangeLevel > level {
			level = pull.ChangeLevel
		}
	}
	return level
}

func (c gitCommit) pullsLabeledStable() []ghPull {
	var result []ghPull
	for _, p := range c.Pulls {
		if p.HasStableLabel {
			result = append(result, p)
		}
	}
	return result
}

func (c gitCommit) pullsLabeledPre() []ghPull {
	var result []ghPull
	for _, p := range c.Pulls {
		if p.HasPreLabel {
			result = append(result, p)
		}
	}
	return result
}

func (c gitCommit) pullsWithPrefix() []ghPull {
	var result []ghPull
	for _, p := range c.Pulls {
		if p.PreReleasePrefix != "" {
			result = append(result, p)
		}
	}
	return result
}

func (c gitCommit) validate() error {
	prePulls := c.pullsLabeledPre()
	stablePulls := c.pullsLabeledStable()
	if len(prePulls) > 0 && len(stablePulls) > 0 {
		return fmt.Errorf("commit %s has both stable and prerelease labels: stable PR: %v, prerelease PR: %v", c.Sha, stablePulls, prePulls)
	}
	prefixPulls := c.pullsWithPrefix()
	if len(prefixPulls) > 1 {
		for i := 1; i < len(prefixPulls); i++ {
			if prefixPulls[i].PreReleasePrefix != prefixPulls[0].PreReleasePrefix {
				return fmt.Errorf("commit %s has pull requests with conflicting prefixes: %v and %v", c.Sha, prefixPulls[0], prefixPulls[i])
			}
		}
	}

	missingLabelPR := len(c.Pulls) > 0
	for _, pull := range c.Pulls {
		if len(pull.LevelLabels) > 0 || pull.HasStableLabel {
			missingLabelPR = false
			break
		}
	}
	if missingLabelPR {
		return fmt.Errorf("commit %s has no labels on associated pull requests: %v", c.Sha, c.Pulls)
	}
	return nil
}
