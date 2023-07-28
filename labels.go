package main

import (
	"strings"

	"golang.org/x/exp/maps"
)

const (
	labelNone       = "semver:none"
	labelPatch      = "semver:patch"
	labelMinor      = "semver:minor"
	labelBreaking   = "semver:breaking"
	labelStable     = "semver:stable"
	labelPrerelease = "semver:prerelease"
)

var labelLevels = map[string]changeLevel{
	labelBreaking: changeLevelMajor,
	labelMinor:    changeLevelMinor,
	labelPatch:    changeLevelPatch,
	labelNone:     changeLevelNone,
}

func normalizeAliases(aliases map[string]string) map[string]string {
	clone := maps.Clone(aliases)
	for k, v := range clone {
		clone[strings.ToLower(k)] = v
	}
	return clone
}

// checkPrereleaseLabel returns true if the label is a prerelease label and the prerelease prefix (the part after the final colon)
func checkPrereleaseLabel(label string, aliases map[string]string) (pre bool, prefix string) {
	downcased := strings.ToLower(label)
	if downcased == labelPrerelease {
		return true, ""
	}
	preLabel := ""
	if strings.HasPrefix(downcased, labelPrerelease+":") {
		preLabel = labelPrerelease + ":"
	} else {
		for alias, target := range aliases {
			alias = strings.ToLower(alias)
			if target != labelPrerelease {
				continue
			}
			if downcased == alias {
				return true, ""
			}
			if strings.HasPrefix(label, alias+":") {
				preLabel = alias + ":"
				break
			}
		}
	}
	if preLabel == "" {
		return false, ""
	}
	return true, strings.TrimPrefix(label, preLabel)
}

func ResolveLabel(label string, aliases map[string]string) string {
	label = strings.ToLower(label)
	_, ok := labelLevels[label]
	if ok {
		return label
	}
	if label == labelStable {
		return label
	}
	if aliases == nil {
		return ""
	}
	v := normalizeAliases(aliases)[label]
	if v == labelPrerelease {
		v = ""
	}
	return v
}
