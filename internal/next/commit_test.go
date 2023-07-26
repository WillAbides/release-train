package next

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train/v3/internal"
)

func testCommit(t *testing.T, pulls ...internal.BasePull) Commit {
	t.Helper()
	c := Commit{Sha: "deadbeef"}
	for i := range pulls {
		p, err := internal.NewPull(pulls[i].Number, nil, pulls[i].Labels...)
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
			commit: testCommit(t, basePull(1, internal.LabelBreaking), basePull(2, internal.LabelNone)),
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
			commit: testCommit(t, basePull(1, internal.LabelPrerelease)),
			err:    `commit deadbeef has no labels on associated pull requests: [#1]`,
		},
		{
			name:   "prerelease",
			commit: testCommit(t, basePull(1, internal.LabelPrerelease, internal.LabelMinor)),
		},
		{
			name:   "one prerelease",
			commit: testCommit(t, basePull(1, internal.LabelPrerelease, internal.LabelMinor), basePull(2, internal.LabelMinor)),
		},
		{
			name:   "stable",
			commit: testCommit(t, basePull(1, internal.LabelStable, internal.LabelMinor)),
		},
		{
			name:   "one stable",
			commit: testCommit(t, basePull(1, internal.LabelStable, internal.LabelMinor), basePull(2, internal.LabelMinor)),
		},
		{
			name:   "stable and prerelease",
			commit: testCommit(t, basePull(1, internal.LabelStable, internal.LabelNone), basePull(2, internal.LabelPrerelease, internal.LabelMinor)),
			err:    `commit deadbeef has both stable and prerelease labels: stable PR: [#1], prerelease PR: [#2]`,
		},
		{
			name:   "prerelease prefix",
			commit: testCommit(t, basePull(1, internal.LabelMinor, internal.LabelPrerelease+":foo")),
		},
		{
			name:   "prerelease with and without prefix",
			commit: testCommit(t, basePull(1, internal.LabelMinor, internal.LabelPrerelease+":foo", internal.LabelPrerelease)),
		},
		{
			name:   "conflicting prefixes",
			commit: testCommit(t, basePull(1, internal.LabelPrerelease+":foo"), basePull(2, internal.LabelPrerelease+":bar")),
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
