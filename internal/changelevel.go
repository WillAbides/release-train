package internal

import (
	"fmt"
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

func Ptr[V any](v V) *V {
	return &v
}
