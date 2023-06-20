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

var (
	PrereleaseLabels = []string{LabelPrerelease}
	StableLabels     = []string{LabelStable}
)

// CheckPrereleaseLabel returns true if the label is a prerelease label and the prerelease prefix (the part after the final colon)
func CheckPrereleaseLabel(label string) (pre bool, prefix string) {
	for _, l := range PrereleaseLabels {
		if label == l {
			return true, ""
		}
		if strings.HasPrefix(label, l+":") {
			return true, label[len(l)+1:]
		}
	}
	return false, ""
}

func CheckStableLabel(label string) bool {
	for _, l := range StableLabels {
		if label == l {
			return true
		}
	}
	return false
}
