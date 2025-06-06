package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPullWithOptions(t *testing.T) {
	t.Run("force prerelease with semver label", func(t *testing.T) {
		pull, err := newPull(123, nil, true, "semver:minor")
		require.NoError(t, err)
		require.True(t, pull.HasPreLabel, "should have prerelease label when force prerelease is enabled and has semver label")
		require.Equal(t, changeLevelMinor, pull.ChangeLevel)
		require.Equal(t, []string{"semver:minor"}, pull.LevelLabels)
	})

	t.Run("force prerelease without semver label", func(t *testing.T) {
		pull, err := newPull(123, nil, true, "documentation")
		require.NoError(t, err)
		require.False(t, pull.HasPreLabel, "should not have prerelease label when no semver label present")
		require.Equal(t, changeLevelNone, pull.ChangeLevel)
		require.Empty(t, pull.LevelLabels)
	})

	t.Run("force prerelease with existing prerelease label", func(t *testing.T) {
		pull, err := newPull(123, nil, true, "semver:minor", "semver:prerelease")
		require.NoError(t, err)
		require.True(t, pull.HasPreLabel, "should keep existing prerelease label")
		require.Equal(t, changeLevelMinor, pull.ChangeLevel)
		require.Equal(t, []string{"semver:minor"}, pull.LevelLabels)
	})

	t.Run("no force prerelease", func(t *testing.T) {
		pull, err := newPull(123, nil, false, "semver:minor")
		require.NoError(t, err)
		require.False(t, pull.HasPreLabel, "should not have prerelease label when force prerelease is disabled")
		require.Equal(t, changeLevelMinor, pull.ChangeLevel)
		require.Equal(t, []string{"semver:minor"}, pull.LevelLabels)
	})

	t.Run("force prerelease with multiple semver labels", func(t *testing.T) {
		pull, err := newPull(123, nil, true, "semver:minor", "semver:patch", "documentation")
		require.NoError(t, err)
		require.True(t, pull.HasPreLabel, "should have prerelease label when force prerelease is enabled and has semver labels")
		require.Equal(t, changeLevelMinor, pull.ChangeLevel) // minor is higher than patch
		require.Equal(t, []string{"semver:minor", "semver:patch"}, pull.LevelLabels)
	})
}
