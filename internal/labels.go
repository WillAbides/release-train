package internal

import (
	"strings"

	"golang.org/x/exp/maps"
)

const (
	LabelNone       = "semver:none"
	LabelPatch      = "semver:patch"
	LabelMinor      = "semver:minor"
	LabelBreaking   = "semver:breaking"
	LabelStable     = "semver:stable"
	LabelPrerelease = "semver:prerelease"
)

var LabelLevels = map[string]ChangeLevel{
	LabelBreaking: ChangeLevelMajor,
	LabelMinor:    ChangeLevelMinor,
	LabelPatch:    ChangeLevelPatch,
	LabelNone:     ChangeLevelNone,
}

func normalizeAliases(aliases map[string]string) map[string]string {
	clone := maps.Clone(aliases)
	for k, v := range clone {
		clone[strings.ToLower(k)] = v
	}
	return clone
}

// CheckPrereleaseLabel returns true if the label is a prerelease label and the prerelease prefix (the part after the final colon)
func CheckPrereleaseLabel(label string, aliases map[string]string) (pre bool, prefix string) {
	downcased := strings.ToLower(label)
	if downcased == LabelPrerelease {
		return true, ""
	}
	preLabel := ""
	if strings.HasPrefix(downcased, LabelPrerelease+":") {
		preLabel = LabelPrerelease + ":"
	} else {
		for alias, target := range aliases {
			alias = strings.ToLower(alias)
			if target != LabelPrerelease {
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
	_, ok := LabelLevels[label]
	if ok {
		return label
	}
	if label == LabelStable {
		return label
	}
	if aliases == nil {
		return ""
	}
	v := normalizeAliases(aliases)[label]
	if v == LabelPrerelease {
		v = ""
	}
	return v
}
