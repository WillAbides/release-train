package main

import (
	"fmt"
	"log/slog"

	"github.com/Masterminds/semver/v3"
)

// versionChange represents a version change, including previous and next versions and the change level.
type versionChange struct {
	NextVersion     semver.Version `json:"next_version"`
	PreviousVersion semver.Version `json:"previous_version"`
	ChangeLevel     changeLevel    `json:"change_level"`
}

// calculateVersionChange determines the next version based on the constraints and commits provided.
func calculateVersionChange(
	previousVersion semver.Version,
	minChange, maxChange changeLevel,
	commits gitCommits,
	forcePrerelease, forceStable bool,
) (*versionChange, error) {
	pulls := commits.pulls()

	// No pulls means no changes, just handle forceStable if needed
	if len(pulls) == 0 {
		nextVersion := previousVersion
		if forceStable {
			nextVersion = removePrerelease(nextVersion)
		}
		return &versionChange{
			PreviousVersion: previousVersion,
			NextVersion:     nextVersion,
			ChangeLevel:     changeLevelNone,
		}, nil
	}

	level := commits.changeLevel(minChange, maxChange)

	isPrerelease := forcePrerelease || (!forceStable && len(pulls.prerelease()) > 0)

	err := pulls.validateForChange(previousVersion, isPrerelease, forceStable, forcePrerelease)
	if err != nil {
		return nil, err
	}

	if isPrerelease {
		return calculatePrereleaseChange(previousVersion, level, commits)
	}

	nextVersion := previousVersion

	// If already stable, just increment normally
	if previousVersion.Prerelease() == "" {
		return &versionChange{
			PreviousVersion: previousVersion,
			NextVersion:     level.incVersion(nextVersion),
			ChangeLevel:     level,
		}, nil
	}

	// Transitioning from prerelease to stable version
	nextVersion = removePrerelease(nextVersion)
	slog.Debug("made stable from pre-release", slog.String("baseVersion", nextVersion.String()))

	// Special case: when forcing stable from prerelease with unstable PRs,
	// don't increment the version number - just remove the prerelease suffix.
	if forceStable && len(pulls.unstable()) > 0 {
		slog.Debug(
			"forceStable from pre-release with unstable PRs: version number will not be incremented",
			slog.Any("unstablePulls", pulls.unstable()),
			slog.String("nextVersion", nextVersion.String()),
		)
		return &versionChange{
			PreviousVersion: previousVersion,
			NextVersion:     nextVersion,
			ChangeLevel:     level,
		}, nil
	}

	// Normal case: increment version when transitioning to stable
	nextVersion = level.incVersion(nextVersion)
	return &versionChange{
		PreviousVersion: previousVersion,
		NextVersion:     nextVersion,
		ChangeLevel:     level,
	}, nil
}

// calculatePrereleaseChange creates a new pre-release version based on the previous version and change level.
func calculatePrereleaseChange(
	prev semver.Version,
	level changeLevel,
	commits gitCommits,
) (*versionChange, error) {
	prefix, err := commits.pulls().prereleasePrefix()
	if err != nil {
		return nil, err
	}
	nextVersion, err := level.incPrerelease(prev, prefix)
	if err != nil {
		return nil, err
	}
	if !nextVersion.GreaterThan(&prev) {
		return nil, fmt.Errorf("pre-release version %q is not greater than %q", nextVersion, prev)
	}
	return &versionChange{
		PreviousVersion: prev,
		NextVersion:     nextVersion,
		ChangeLevel:     level,
	}, nil
}

// removePrerelease strips the prerelease suffix from a version.
func removePrerelease(version semver.Version) semver.Version {
	version, err := version.SetPrerelease("")
	if err != nil {
		panic("empty prerelease is always valid")
	}
	return version
}
