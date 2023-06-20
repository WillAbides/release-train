package internal

import (
	"strings"
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

// CheckPrereleaseLabel returns true if the label is a prerelease label and the prerelease prefix (the part after the final colon)
func CheckPrereleaseLabel(label string, aliases map[string]string) (pre bool, prefix string) {
	if label == LabelPrerelease {
		return true, ""
	}
	preLabel := ""
	if strings.HasPrefix(label, LabelPrerelease+":") {
		preLabel = LabelPrerelease + ":"
	}
	for k, v := range aliases {
		if preLabel != "" {
			break
		}
		if v != LabelPrerelease {
			continue
		}
		if label == k {
			return true, ""
		}
		if strings.HasPrefix(label, k+":") {
			preLabel = k + ":"
		}
	}
	if preLabel == "" {
		return false, ""
	}
	return true, strings.TrimPrefix(label, preLabel)
}

func ResolveLabel(label string, aliases map[string]string) string {
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
	v := aliases[label]
	if v != "" && v != LabelPrerelease {
		return v
	}
	return ""
}

func CheckStableLabel(label string, aliases map[string]string) bool {
	if label == LabelStable {
		return true
	}
	for k, v := range aliases {
		if v == LabelStable && label == k {
			return true
		}
	}
	return false
}
