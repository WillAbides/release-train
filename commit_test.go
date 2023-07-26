package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testCommit(t *testing.T, pulls ...BasePull) gitCommit {
	t.Helper()
	c := gitCommit{Sha: "deadbeef"}
	for i := range pulls {
		p, err := newPull(pulls[i].Number, nil, pulls[i].Labels...)
		require.NoError(t, err)
		c.Pulls = append(c.Pulls, *p)
	}
	return c
}

func newBasePull(number int, labels ...string) BasePull {
	return BasePull{
		Number: number,
		Labels: labels,
	}
}

func TestCommit_validate(t *testing.T) {
	for _, td := range []struct {
		name   string
		commit gitCommit
		err    string
	}{
		{
			name:   "no pulls",
			commit: testCommit(t),
		},
		{
			name:   "all labeled",
			commit: testCommit(t, newBasePull(1, labelBreaking), newBasePull(2, labelNone)),
		},
		{
			name:   "unlabeled",
			commit: testCommit(t, newBasePull(1)),
			err:    `commit deadbeef has no labels on associated pull requests: [#1]`,
		},
		{
			name:   "multiple unlabeled",
			commit: testCommit(t, newBasePull(2), newBasePull(1)),
			err:    `commit deadbeef has no labels on associated pull requests: [#2 #1]`,
		},
		{
			name:   "unlabeled with prerelease",
			commit: testCommit(t, newBasePull(1, labelPrerelease)),
			err:    `commit deadbeef has no labels on associated pull requests: [#1]`,
		},
		{
			name:   "prerelease",
			commit: testCommit(t, newBasePull(1, labelPrerelease, labelMinor)),
		},
		{
			name:   "one prerelease",
			commit: testCommit(t, newBasePull(1, labelPrerelease, labelMinor), newBasePull(2, labelMinor)),
		},
		{
			name:   "stable",
			commit: testCommit(t, newBasePull(1, labelStable, labelMinor)),
		},
		{
			name:   "one stable",
			commit: testCommit(t, newBasePull(1, labelStable, labelMinor), newBasePull(2, labelMinor)),
		},
		{
			name:   "stable and prerelease",
			commit: testCommit(t, newBasePull(1, labelStable, labelNone), newBasePull(2, labelPrerelease, labelMinor)),
			err:    `commit deadbeef has both stable and prerelease labels: stable PR: [#1], prerelease PR: [#2]`,
		},
		{
			name:   "prerelease prefix",
			commit: testCommit(t, newBasePull(1, labelMinor, labelPrerelease+":foo")),
		},
		{
			name:   "prerelease with and without prefix",
			commit: testCommit(t, newBasePull(1, labelMinor, labelPrerelease+":foo", labelPrerelease)),
		},
		{
			name:   "conflicting prefixes",
			commit: testCommit(t, newBasePull(1, labelPrerelease+":foo"), newBasePull(2, labelPrerelease+":bar")),
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
