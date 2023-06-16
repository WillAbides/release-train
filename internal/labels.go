package internal

import (
	"strings"
)

var LabelLevels = map[string]ChangeLevel{
	"breaking":        ChangeLevelMajor,
	"breaking change": ChangeLevelMajor,
	"major":           ChangeLevelMajor,
	"semver:major":    ChangeLevelMajor,

	"enhancement":  ChangeLevelMinor,
	"minor":        ChangeLevelMinor,
	"semver:minor": ChangeLevelMinor,

	"bug":          ChangeLevelPatch,
	"fix":          ChangeLevelPatch,
	"patch":        ChangeLevelPatch,
	"semver:patch": ChangeLevelPatch,

	"no change":        ChangeLevelNoChange,
	"semver:none":      ChangeLevelNoChange,
	"semver:no change": ChangeLevelNoChange,
	"semver:nochange":  ChangeLevelNoChange,
	"semver:skip":      ChangeLevelNoChange,
}

var (
	PrereleaseLabels = []string{"semver:pre", "semver:prerelease", "prerelease"}
	StableLabels     = []string{"semver:stable"}
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
