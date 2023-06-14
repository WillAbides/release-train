package internal

import (
	"fmt"
	"strings"
)

type ChangeLevel int

const (
	ChangeLevelNoChange ChangeLevel = iota
	ChangeLevelPatch
	ChangeLevelMinor
	ChangeLevelMajor
)

func (l ChangeLevel) String() string {
	switch l {
	case ChangeLevelNoChange:
		return "no change"
	case ChangeLevelPatch:
		return "patch"
	case ChangeLevelMinor:
		return "minor"
	case ChangeLevelMajor:
		return "major"
	default:
		panic("invalid change level")
	}
}

func (l ChangeLevel) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", l.String())), nil
}

func ParseChangeLevel(v string) (ChangeLevel, error) {
	switch strings.ToLower(v) {
	case "patch":
		return ChangeLevelPatch, nil
	case "minor":
		return ChangeLevelMinor, nil
	case "major":
		return ChangeLevelMajor, nil
	case "none", "no change":
		return ChangeLevelNoChange, nil
	default:
		return ChangeLevelNoChange, fmt.Errorf("invalid change level: %s", v)
	}
}
