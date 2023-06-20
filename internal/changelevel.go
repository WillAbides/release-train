package internal

import (
	"fmt"
	"strings"
)

type ChangeLevel int

const (
	ChangeLevelNone ChangeLevel = iota
	ChangeLevelPatch
	ChangeLevelMinor
	ChangeLevelMajor
)

func (l ChangeLevel) String() string {
	switch l {
	case ChangeLevelNone:
		return "none"
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
	case "none":
		return ChangeLevelNone, nil
	default:
		return 0, fmt.Errorf("invalid change level: %s", v)
	}
}
