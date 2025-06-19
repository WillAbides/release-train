package main

import (
	"cmp"
	"fmt"
	"slices"
	"sort"

	"github.com/Masterminds/semver/v3"
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
		level, ok := labelLevel(resolvedLabel)
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

type ghPulls []ghPull

func (p ghPulls) filter(filter func(ghPull) bool) ghPulls {
	var result ghPulls
	for _, pull := range p {
		if filter(pull) {
			result = append(result, pull)
		}
	}
	return result
}

// stable returns pulls that have the stable label.
func (p ghPulls) stable() ghPulls {
	return p.filter(func(pull ghPull) bool {
		return pull.HasStableLabel
	})
}

// unstable returns pulls that do not have the stable label.
func (p ghPulls) unstable() ghPulls {
	return p.filter(func(pull ghPull) bool {
		return !pull.HasStableLabel
	})
}

// prerelease returns pulls that have the prerelease label.
func (p ghPulls) prerelease() ghPulls {
	return p.filter(func(pull ghPull) bool {
		return pull.HasPreLabel
	})
}

// nonPrerelease returns pulls that do not indicate a prerelease.
func (p ghPulls) nonPrerelease() ghPulls {
	return p.filter(func(pull ghPull) bool {
		return !pull.HasPreLabel && pull.ChangeLevel > changeLevelNone
	})
}

// compact sorts and removes duplicate pulls based on their number.
func (p ghPulls) compact() ghPulls {
	pulls := slices.Clone(p)
	slices.SortFunc(pulls, func(a, b ghPull) int {
		return cmp.Compare(a.Number, b.Number)
	})
	return slices.CompactFunc(pulls, func(a, b ghPull) bool {
		return a.Number == b.Number
	})
}

// prereleasePrefix returns the pre-release prefix if all pre-release pulls have the same prefix.
func (p ghPulls) prereleasePrefix() (string, error) {
	prefixPulls := p.prerelease().filter(func(pull ghPull) bool { return pull.PreReleasePrefix != "" })
	prefix := ""
	for _, pull := range prefixPulls {
		if prefix != "" && prefix != pull.PreReleasePrefix {
			return "", fmt.Errorf(
				"cannot have multiple pre-release prefixes in the same release. release contains both %q and %q",
				prefix, pull.PreReleasePrefix,
			)
		}
		prefix = pull.PreReleasePrefix
	}
	return prefix, nil
}

func (p ghPulls) validateForChange(version semver.Version, isPrerelease, forceStable, forcePrerelease bool) error {
	if len(p.prerelease()) > 0 && len(p.nonPrerelease()) > 0 {
		return fmt.Errorf("cannot have pre-release and non-pre-release PRs in the same release. pre-release PRs: %v, non-pre-release PRs: %v",
			p.prerelease(), p.nonPrerelease())
	}

	stablePulls := p.stable()
	if forcePrerelease && len(stablePulls) > 0 {
		return fmt.Errorf("cannot force pre-release with stable PRs. stable PRs: %v", stablePulls)
	}

	// the rest of this only applies to transitioning from prerelease to stable
	if isPrerelease || version.Prerelease() == "" || forceStable {
		// Already stable, no need to validate labels
		return nil
	}

	unstablePulls := p.unstable()
	if len(unstablePulls) > 0 && len(stablePulls) == 0 {
		return fmt.Errorf(
			"cannot create a stable release from a pre-release unless all PRs are labeled semver:stable. unlabeled PRs: %v",
			unstablePulls,
		)
	}

	if len(stablePulls) > 0 && len(unstablePulls) > 0 {
		return fmt.Errorf(
			"in order to release a stable version, all PRs must be labeled as stable. stable PRs: %v, unstable PRs: %v",
			stablePulls, unstablePulls,
		)
	}

	return nil
}
