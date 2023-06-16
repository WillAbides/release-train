package next

import (
	"fmt"

	"github.com/willabides/release-train-action/v2/internal"
)

type Commit struct {
	Sha   string          `json:"sha"`
	Pulls []internal.Pull `json:"pulls,omitempty"`
}

func (c Commit) changeLevel() internal.ChangeLevel {
	level := internal.ChangeLevelNoChange
	for _, pull := range c.Pulls {
		if pull.ChangeLevel > level {
			level = pull.ChangeLevel
		}
	}
	return level
}

func (c Commit) pullsLabeledStable() []internal.Pull {
	var result []internal.Pull
	for _, p := range c.Pulls {
		if p.HasStableLabel {
			result = append(result, p)
		}
	}
	return result
}

func (c Commit) pullsLabeledPre() []internal.Pull {
	var result []internal.Pull
	for _, p := range c.Pulls {
		if p.HasPreLabel {
			result = append(result, p)
		}
	}
	return result
}

func (c Commit) pullsWithPrefix() []internal.Pull {
	var result []internal.Pull
	for _, p := range c.Pulls {
		if p.PreReleasePrefix != "" {
			result = append(result, p)
		}
	}
	return result
}

func (c Commit) validate() error {
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
		if len(pull.LevelLabels) > 0 {
			missingLabelPR = false
			break
		}
	}
	if missingLabelPR {
		return fmt.Errorf("commit %s has no labels on associated pull requests: %v", c.Sha, c.Pulls)
	}
	return nil
}
