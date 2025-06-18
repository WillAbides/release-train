package main

import (
	"cmp"
	"fmt"
)

type gitCommit struct {
	Sha   string  `json:"sha"`
	Pulls ghPulls `json:"pulls,omitempty"`
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

func (c gitCommit) validate() error {
	prePulls := c.Pulls.filter(func(pull ghPull) bool { return pull.HasPreLabel })
	stablePulls := c.Pulls.stable()

	if len(prePulls) > 0 && len(stablePulls) > 0 {
		return fmt.Errorf("commit %s has both stable and prerelease labels: stable PR: %v, prerelease PR: %v", c.Sha, stablePulls, prePulls)
	}
	prefixPulls := c.Pulls.filter(func(pull ghPull) bool { return pull.PreReleasePrefix != "" })
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

type gitCommits []gitCommit

func (c gitCommits) pulls() ghPulls {
	var pulls []ghPull
	for _, commit := range c {
		pulls = append(pulls, commit.Pulls...)
	}
	return ghPulls(pulls).compact()
}

func (c gitCommits) changeLevel(minChange, maxChange changeLevel) changeLevel {
	if len(c) == 0 {
		return changeLevelNone
	}

	maxChange = cmp.Or(maxChange, changeLevelMajor)
	level := minChange
	for _, commit := range c {
		commitLevel := min(commit.changeLevel(), maxChange)
		level = max(level, commitLevel)
	}

	return level
}
