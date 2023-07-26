package main

import (
	"fmt"
)

type changeLevel int

const (
	changeLevelNone changeLevel = iota
	changeLevelPatch
	changeLevelMinor
	changeLevelMajor
)

func (l changeLevel) String() string {
	switch l {
	case changeLevelNone:
		return "none"
	case changeLevelPatch:
		return "patch"
	case changeLevelMinor:
		return "minor"
	case changeLevelMajor:
		return "major"
	default:
		panic("invalid change level")
	}
}

func (l changeLevel) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", l.String())), nil
}
