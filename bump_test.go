package main

import (
	"context"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"
)

func Test_bumpVersion(t *testing.T) {
	pull := func(
		number int,
		level changeLevel,
		hasPreLabel, hasStableLabel bool,
		prefix string,
	) ghPull {
		return ghPull{
			Number:           number,
			ChangeLevel:      level,
			HasPreLabel:      hasPreLabel,
			HasStableLabel:   hasStableLabel,
			PreReleasePrefix: prefix,
		}
	}

	commit := func(pulls ...ghPull) gitCommit {
		return gitCommit{Pulls: pulls}
	}

	for _, td := range []struct {
		name string

		prev            string
		minBump         changeLevel
		maxBump         changeLevel
		commits         []gitCommit
		forcePrerelease bool
		forceStable     bool

		want    *getNextResult
		wantErr string
	}{
		{
			name: "no commits",
			prev: "1.2.3",
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.3"),
				PreviousVersion: *semver.MustParse("1.2.3"),
			},
		},
		{
			name: "no commits, prerelease",
			prev: "1.2.3-alpha.0",
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.3-alpha.0"),
				PreviousVersion: *semver.MustParse("1.2.3-alpha.0"),
			},
		},
		{
			name: "bump stable",
			prev: "1.2.3-2",
			commits: []gitCommit{
				commit(
					pull(1, changeLevelPatch, false, true, ""),
					pull(2, changeLevelMinor, false, true, ""),
				),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.3.0"),
				PreviousVersion: *semver.MustParse("1.2.3-2"),
				ChangeLevel:     changeLevelMinor,
			},
		},
		{
			name: "new prerelease",
			prev: "1.2.3",
			commits: []gitCommit{
				commit(pull(1, changeLevelPatch, true, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.4-0"),
				PreviousVersion: *semver.MustParse("1.2.3"),
				ChangeLevel:     changeLevelPatch,
			},
		},
		{
			name:            "force prerelease",
			prev:            "1.2.3",
			forcePrerelease: true,
			commits: []gitCommit{
				commit(pull(1, changeLevelPatch, false, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.4-0"),
				PreviousVersion: *semver.MustParse("1.2.3"),
				ChangeLevel:     changeLevelPatch,
			},
		},
		{
			name: "bump prerelease using previous prefix",
			prev: "1.2.3-alpha.33",
			commits: []gitCommit{
				commit(
					pull(1, changeLevelPatch, true, false, "alpha"),
					pull(2, changeLevelNone, true, false, "alpha"),
				),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.3-alpha.34"),
				PreviousVersion: *semver.MustParse("1.2.3-alpha.33"),
				ChangeLevel:     changeLevelPatch,
			},
		},
		{
			name:            "force prerelease with previous prefix",
			prev:            "1.2.3-alpha.33",
			forcePrerelease: true,
			commits: []gitCommit{
				commit(
					pull(1, changeLevelPatch, false, false, "alpha"),
					pull(2, changeLevelNone, false, false, "alpha"),
				),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.3-alpha.34"),
				PreviousVersion: *semver.MustParse("1.2.3-alpha.33"),
				ChangeLevel:     changeLevelPatch,
			},
		},
		{
			name: "mixed prefixes",
			prev: "1.2.3",
			commits: []gitCommit{
				commit(
					pull(1, changeLevelPatch, true, false, "alpha"),
					pull(2, changeLevelNone, true, false, "beta"),
				),
			},
			wantErr: `cannot have multiple pre-release prefixes in the same release. pre-release prefix. release contains both "alpha" and "beta"`,
		},
		{
			name: "mixed prerelease and non-prerelease on stable",
			prev: "1.2.3",
			commits: []gitCommit{
				commit(
					pull(1, changeLevelPatch, true, false, ""),
					pull(2, changeLevelPatch, false, true, ""),
				),
			},
			wantErr: "cannot have pre-release and non-pre-release PRs in the same release. pre-release PRs: [#1], non-pre-release PRs: [#2]",
		},
		{
			name: "mixed prerelease and non-prerelease on prerelease",
			prev: "1.2.3-0",
			commits: []gitCommit{
				commit(
					pull(1, changeLevelPatch, true, false, ""),
					pull(2, changeLevelPatch, false, true, ""),
				),
			},
			wantErr: "cannot have pre-release and non-pre-release PRs in the same release. pre-release PRs: [#1], non-pre-release PRs: [#2]",
		},
		{
			name: "stable tag only",
			prev: "0.1.0-0",
			commits: []gitCommit{
				commit(pull(1, changeLevelNone, false, true, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("0.1.0"),
				PreviousVersion: *semver.MustParse("0.1.0-0"),
				ChangeLevel:     changeLevelNone,
			},
		},
		{
			name:            "force prerelease with stable tag",
			prev:            "0.1.0",
			forcePrerelease: true,
			commits: []gitCommit{
				commit(pull(1, changeLevelNone, false, true, "")),
			},
			wantErr: `cannot force pre-release with stable PRs. stable PRs: [#1]`,
		},
		{
			name: "prerelease major bump from stable",
			prev: "1.2.3",
			commits: []gitCommit{
				commit(pull(1, changeLevelMajor, true, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("2.0.0-0"),
				PreviousVersion: *semver.MustParse("1.2.3"),
				ChangeLevel:     changeLevelMajor,
			},
		},
		{
			name: "prerelease minor bump from alpha prerelease",
			prev: "1.0.0-alpha.0",
			commits: []gitCommit{
				commit(pull(1, changeLevelMinor, true, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.0.0-alpha.1"),
				PreviousVersion: *semver.MustParse("1.0.0-alpha.0"),
				ChangeLevel:     changeLevelMinor,
			},
		},
		{
			name: "prerelease minor bump from numbered prerelease",
			prev: "1.0.0-0",
			commits: []gitCommit{
				commit(pull(1, changeLevelMinor, true, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.0.0-1"),
				PreviousVersion: *semver.MustParse("1.0.0-0"),
				ChangeLevel:     changeLevelMinor,
			},
		},
		{
			name: "prerelease minor bump requiring version increment",
			prev: "1.0.1-0",
			commits: []gitCommit{
				commit(pull(1, changeLevelMinor, true, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.1.0-0"),
				PreviousVersion: *semver.MustParse("1.0.1-0"),
				ChangeLevel:     changeLevelMinor,
			},
		},
		{
			name: "prerelease patch bump from numbered prerelease",
			prev: "1.0.1-0",
			commits: []gitCommit{
				commit(pull(1, changeLevelPatch, true, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.0.1-1"),
				PreviousVersion: *semver.MustParse("1.0.1-0"),
				ChangeLevel:     changeLevelPatch,
			},
		},
		{
			name: "prerelease major bump from numbered prerelease",
			prev: "1.0.1-0",
			commits: []gitCommit{
				commit(pull(1, changeLevelMajor, true, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("2.0.0-0"),
				PreviousVersion: *semver.MustParse("1.0.1-0"),
				ChangeLevel:     changeLevelMajor,
			},
		},
		{
			name: "prerelease major bump with alpha prefix",
			prev: "1.0.1-0",
			commits: []gitCommit{
				commit(pull(1, changeLevelMajor, true, false, "alpha")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("2.0.0-alpha.0"),
				PreviousVersion: *semver.MustParse("1.0.1-0"),
				ChangeLevel:     changeLevelMajor,
			},
		},
		{
			name: "prerelease none change level error",
			prev: "1.2.3",
			commits: []gitCommit{
				commit(pull(1, changeLevelNone, true, false, "alpha")),
			},
			wantErr: `invalid change level for pre-release: none`,
		},
		{
			name: "prerelease prefix conflict with existing beta",
			prev: "1.2.3-beta.0",
			commits: []gitCommit{
				commit(pull(1, changeLevelPatch, true, false, "alpha")),
			},
			wantErr: `pre-release version "1.2.3-alpha.0" is not greater than "1.2.3-beta.0"`,
		},
		{
			name: "prerelease patch bump from beta with no prefix specified",
			prev: "1.2.3-beta.0",
			commits: []gitCommit{
				commit(pull(1, changeLevelPatch, true, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.3-beta.1"),
				PreviousVersion: *semver.MustParse("1.2.3-beta.0"),
				ChangeLevel:     changeLevelPatch,
			},
		},
		{
			name: "prerelease patch bump from rc with no numeric suffix",
			prev: "1.2.3-rc0",
			commits: []gitCommit{
				commit(pull(1, changeLevelPatch, true, false, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.3-rc0.0"),
				PreviousVersion: *semver.MustParse("1.2.3-rc0"),
				ChangeLevel:     changeLevelPatch,
			},
		},
		{
			name: "prerelease prefix conflict with rc0",
			prev: "1.2.3-rc0",
			commits: []gitCommit{
				commit(pull(1, changeLevelPatch, true, false, "alpha")),
			},
			wantErr: `pre-release version "1.2.3-alpha.0" is not greater than "1.2.3-rc0"`,
		},
		{
			name: "mix stable and pre",
			prev: "1.2.3-alpha.0",
			commits: []gitCommit{
				commit(
					pull(1, changeLevelPatch, false, false, ""),
					pull(2, changeLevelPatch, false, true, ""),
				),
			},
			wantErr: `in order to release a stable version, all PRs must be labeled as stable. stable PRs: [#2], unstable PRs: [#1]`,
		},
		{
			name: "no stable labels on previous prerelease",
			prev: "2.0.0-beta.1",
			commits: []gitCommit{
				commit(pull(1, changeLevelPatch, false, false, "")),
			},
			wantErr: `cannot create a stable release from a pre-release unless all PRs are labeled semver:stable. unlabeled PRs: [#1]`,
		},
		{
			name:    "minBump enforced when change level is lower",
			prev:    "1.2.3",
			minBump: changeLevelMinor,
			commits: []gitCommit{
				commit(pull(1, changeLevelPatch, false, true, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.3.0"),
				PreviousVersion: *semver.MustParse("1.2.3"),
				ChangeLevel:     changeLevelMinor,
			},
		},
		{
			name:    "maxBump enforced when change level is higher",
			prev:    "1.2.3",
			maxBump: changeLevelPatch,
			commits: []gitCommit{
				commit(pull(1, changeLevelMajor, false, true, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.4"),
				PreviousVersion: *semver.MustParse("1.2.3"),
				ChangeLevel:     changeLevelPatch,
			},
		},
		{
			name:    "minBump with no commits does not apply",
			prev:    "1.2.3",
			minBump: changeLevelMinor,
			commits: []gitCommit{},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.3"),
				PreviousVersion: *semver.MustParse("1.2.3"),
				ChangeLevel:     changeLevelNone,
			},
		},
		{
			name:    "minBump enforced with none change level",
			prev:    "1.2.3",
			minBump: changeLevelPatch,
			commits: []gitCommit{
				commit(pull(1, changeLevelNone, false, true, "")),
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.2.4"),
				PreviousVersion: *semver.MustParse("1.2.3"),
				ChangeLevel:     changeLevelPatch,
			},
		},
	} {
		t.Run(td.name, func(t *testing.T) {
			prev := semver.MustParse(td.prev)
			got, err := bumpVersion(context.Background(), *prev, td.minBump, td.maxBump, td.commits, td.forcePrerelease, td.forceStable)
			if td.wantErr != "" {
				require.EqualError(t, err, td.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, td.want, got)
		})
	}
}
