package next

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train-action/v2/internal"
)

func testCommit(t *testing.T, pulls ...internal.BasePull) Commit {
	t.Helper()
	c := Commit{Sha: "deadbeef"}
	for i := range pulls {
		p, err := internal.NewPull(pulls[i].Number, pulls[i].Labels...)
		require.NoError(t, err)
		c.Pulls = append(c.Pulls, *p)
	}
	return c
}

func basePull(number int, labels ...string) internal.BasePull {
	return internal.BasePull{
		Number: number,
		Labels: labels,
	}
}

func TestCommit_validate(t *testing.T) {
	for _, td := range []struct {
		name   string
		commit Commit
		err    string
	}{
		{
			name:   "no pulls",
			commit: testCommit(t),
		},
		{
			name:   "all labeled",
			commit: testCommit(t, basePull(1, "semver:major"), basePull(2, "semver:none")),
		},
		{
			name:   "unlabeled",
			commit: testCommit(t, basePull(1)),
			err:    `commit deadbeef has no labels on associated pull requests: [#1]`,
		},
		{
			name:   "multiple unlabeled",
			commit: testCommit(t, basePull(2), basePull(1)),
			err:    `commit deadbeef has no labels on associated pull requests: [#2 #1]`,
		},
		{
			name:   "unlabeled with prerelease",
			commit: testCommit(t, basePull(1, "semver:pre")),
			err:    `commit deadbeef has no labels on associated pull requests: [#1]`,
		},
		{
			name:   "prerelease",
			commit: testCommit(t, basePull(1, "semver:pre", "semver:minor")),
		},
		{
			name:   "one prerelease",
			commit: testCommit(t, basePull(1, "semver:pre", "semver:minor"), basePull(2, "semver:minor")),
		},
		{
			name:   "stable",
			commit: testCommit(t, basePull(1, "semver:stable", "semver:minor")),
		},
		{
			name:   "one stable",
			commit: testCommit(t, basePull(1, "semver:stable", "semver:minor"), basePull(2, "semver:minor")),
		},
		{
			name:   "stable and prerelease",
			commit: testCommit(t, basePull(1, "semver:stable", "semver:none"), basePull(2, "semver:pre", "semver:minor")),
			err:    `commit deadbeef has both stable and prerelease labels: stable PR: [#1], prerelease PR: [#2]`,
		},
		{
			name:   "prerelease prefix",
			commit: testCommit(t, basePull(1, "semver:minor", "semver:pre:foo")),
		},
		{
			name:   "prerelease with and without prefix",
			commit: testCommit(t, basePull(1, "semver:minor", "semver:pre:foo", "semver:pre")),
		},
		{
			name:   "conflicting prefixes",
			commit: testCommit(t, basePull(1, "semver:pre:foo"), basePull(2, "semver:pre:bar")),
			err:    `commit deadbeef has pull requests with conflicting prefixes: #1 and #2`,
		},
	} {
		t.Run(td.name, func(t *testing.T) {
			err := td.commit.validate()
			if td.err == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, td.err)
		})
	}
}
