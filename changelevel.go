package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type changeLevel int

const (
	changeLevelNone changeLevel = iota
	changeLevelPatch
	changeLevelMinor
	changeLevelMajor
)

// incVersion runs the appropriate Inc* method on the semver.Version based on the changeLevel.
func (l changeLevel) incVersion(version semver.Version) semver.Version {
	switch l {
	case changeLevelNone:
		return version
	case changeLevelPatch:
		return version.IncPatch()
	case changeLevelMinor:
		return version.IncMinor()
	case changeLevelMajor:
		return version.IncMajor()
	default:
		panic("invalid change level")
	}
}

// incPrerelease increments to a pre-release version based on the changeLevel and pre-release prefix.
func (l changeLevel) incPrerelease(ver semver.Version, prefix string) (semver.Version, error) {
	if l == changeLevelNone {
		return semver.Version{}, fmt.Errorf("invalid change level for pre-release: %v", changeLevelNone)
	}
	if ver.Prerelease() == "" {
		return l.initialPrerelease(ver, prefix)
	}
	return l.incrementPrerelease(ver, prefix)
}

func (l changeLevel) initialPrerelease(ver semver.Version, prefix string) (semver.Version, error) {
	prereleaseTag := "0"
	if prefix != "" {
		prereleaseTag = prefix + ".0"
	}
	// Increment the base version first, then add prerelease suffix
	return l.incVersion(ver).SetPrerelease(prereleaseTag)
}

func (l changeLevel) prereleaseNeedsStableVersionIncrement(version semver.Version) bool {
	// If bumping minor or higher and patch is nonzero, or bumping major and minor is nonzero,
	// a stable version increment is required to reset lower version components.
	if l >= changeLevelMinor && version.Patch() > 0 {
		return true
	}
	return l == changeLevelMajor && version.Minor() > 0
}

func (l changeLevel) incrementPrerelease(version semver.Version, prefix string) (semver.Version, error) {
	prevPrerelease := version.Prerelease()
	// Split the prerelease string into its dot-separated parts (e.g., prefix.1)
	prereleaseParts := strings.Split(prevPrerelease, ".")

	// If a stable version increment is needed, bump the base version and reset the prerelease counter.
	if l.prereleaseNeedsStableVersionIncrement(version) {
		version = l.incVersion(version)
		return buildPrereleaseWithCounter(version, prefix, prereleaseParts, -1)
	}

	// Parse the numeric counter from the last part of the prerelease string.
	prereleaseCount, err := strconv.Atoi(prereleaseParts[len(prereleaseParts)-1])
	if err != nil {
		// Non-numeric suffix - start fresh with .0
		if prefix == "" {
			prefix = prevPrerelease
		}
		return version.SetPrerelease(prefix + ".0")
	}

	return buildPrereleaseWithCounter(version, prefix, prereleaseParts, prereleaseCount)
}

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

//nolint:unparam // error needed to meet json.Marshaler
func (l changeLevel) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", l.String())), nil
}

// buildPrereleaseWithCounter constructs a prerelease string with the correct counter.
func buildPrereleaseWithCounter(
	version semver.Version,
	prefix string,
	existingParts []string,
	counter int,
) (semver.Version, error) {
	existingPrefix := strings.Join(existingParts[:len(existingParts)-1], ".")

	switch {
	case prefix == "" && existingPrefix == "":
		// No prefix - just increment the counter
		return version.SetPrerelease(strconv.Itoa(counter + 1))

	case prefix == existingPrefix || prefix == "":
		// Keep existing prefix
		return version.SetPrerelease(existingPrefix + "." + strconv.Itoa(counter+1))

	default:
		// New prefix - start counter at 0
		return version.SetPrerelease(prefix + ".0")
	}
}
