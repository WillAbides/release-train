package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train/v3/internal/github"
	"go.uber.org/mock/gomock"
)

func Test_incrPre(t *testing.T) {
	for _, td := range []struct {
		prev    string
		level   changeLevel
		prefix  string
		want    string
		wantErr string
	}{
		{
			prev:  "1.2.3",
			level: changeLevelMajor,
			want:  "2.0.0-0",
		},
		{
			prev:  "1.0.0-alpha.0",
			level: changeLevelMinor,
			want:  "1.0.0-alpha.1",
		},
		{
			prev:  "1.0.0-0",
			level: changeLevelMinor,
			want:  "1.0.0-1",
		},
		{
			prev:  "1.0.1-0",
			level: changeLevelMinor,
			want:  "1.1.0-0",
		},
		{
			prev:  "1.0.1-0",
			level: changeLevelPatch,
			want:  "1.0.1-1",
		},
		{
			prev:  "1.0.1-0",
			level: changeLevelMajor,
			want:  "2.0.0-0",
		},
		{
			prev:   "1.0.1-0",
			level:  changeLevelMajor,
			prefix: "alpha",
			want:   "2.0.0-alpha.0",
		},
		{
			prev:    "1.2.3",
			level:   changeLevelNone,
			prefix:  "alpha",
			wantErr: `invalid change level for pre-release: none`,
		},
		{
			prev:    "1.2.3-beta.0",
			level:   changeLevelPatch,
			prefix:  "alpha",
			wantErr: `pre-release version "1.2.3-alpha.0" is not greater than "1.2.3-beta.0"`,
		},
		{
			prev:   "1.2.3-beta.0",
			level:  changeLevelPatch,
			prefix: "",
			want:   "1.2.3-beta.1",
		},
		{
			prev:    "1.2.3-beta.0",
			level:   changeLevelPatch,
			prefix:  "_invalid",
			wantErr: "Invalid Prerelease string",
		},
		{
			prev:   "1.2.3-rc0",
			level:  changeLevelPatch,
			prefix: "",
			want:   "1.2.3-rc0.0",
		},
		{
			prev:    "1.2.3-rc0",
			level:   changeLevelPatch,
			prefix:  "alpha",
			wantErr: `pre-release version "1.2.3-alpha.0" is not greater than "1.2.3-rc0"`,
		},
	} {
		name := fmt.Sprintf("%s-%s-%s", td.prev, td.level, td.prefix)
		t.Run(name, func(t *testing.T) {
			prev := semver.MustParse(td.prev)
			got, err := incrPre(*prev, td.level, td.prefix)
			if td.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), td.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, td.want, got.String())
		})
	}
}

func TestGetNext(t *testing.T) {
	ctx := context.Background()

	sha1 := "1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha3 := "3aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mergeSha := "4aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	tests := []struct {
		name       string
		setupMocks func(*MockGithubClient)
		options    *getNextOptions
		want       *getNextResult
		wantErr    string
	}{
		{
			name: "major",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{
						AheadBy: 2,
						Commits: []string{sha1, sha2},
					}, nil,
				)
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", mergeSha, sha1, 0).Return(
					&github.CommitComparison{AheadBy: 2}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha1).Return(
					[]github.BasePull{
						// non-standard caps to test case insensitivity
						{Number: 1, MergeCommitSha: mergeSha, Labels: []string{"SEMVER:BREAKING", "something else"}},
						{Number: 2, MergeCommitSha: mergeSha, Labels: []string{"something else"}},
						{Number: 3, MergeCommitSha: mergeSha},
						{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelMinor}},
					}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha2).Return(
					[]github.BasePull{}, nil,
				)
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        "v0.15.0",
				PrevVersion: "0.15.0",
				Head:        sha1,
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.0.0"),
				PreviousVersion: *semver.MustParse("0.15.0"),
				ChangeLevel:     changeLevelMajor,
			},
		},
		{
			name: "minor",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{
						AheadBy: 2,
						Commits: []string{sha1, sha2},
					}, nil,
				)
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", mergeSha, sha1, 0).Return(
					&github.CommitComparison{AheadBy: 2}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha1).Return(
					[]github.BasePull{
						{Number: 1, MergeCommitSha: mergeSha, Labels: []string{"something else"}},
						{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelMinor}},
						{Number: 3, MergeCommitSha: mergeSha},
						{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
					}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha2).Return(
					[]github.BasePull{}, nil,
				)
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        "v0.15.0",
				PrevVersion: "0.15.0",
				Head:        sha1,
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("0.16.0"),
				PreviousVersion: *semver.MustParse("0.15.0"),
				ChangeLevel:     changeLevelMinor,
			},
		},
		{
			name: "patch",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{
						AheadBy: 2,
						Commits: []string{sha1, sha2},
					}, nil,
				)
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", mergeSha, sha1, 0).Return(
					&github.CommitComparison{AheadBy: 2}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha1).Return(
					[]github.BasePull{
						{Number: 1, MergeCommitSha: mergeSha, Labels: []string{"something else"}},
						{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
						{Number: 3, MergeCommitSha: mergeSha},
						{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
					}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha2).Return(
					[]github.BasePull{}, nil,
				)
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        "v0.15.0",
				PrevVersion: "0.15.0",
				Head:        sha1,
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("0.15.1"),
				PreviousVersion: *semver.MustParse("0.15.0"),
				ChangeLevel:     changeLevelPatch,
			},
		},
		{
			name: "check pr",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{
						AheadBy: 2,
						Commits: []string{sha1, sha2},
					}, nil,
				)
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", mergeSha, sha1, 0).Return(
					&github.CommitComparison{AheadBy: 2}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha1).Return(
					[]github.BasePull{
						{Number: 1, MergeCommitSha: mergeSha, Labels: []string{"something else"}},
						{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
						{Number: 3, MergeCommitSha: mergeSha},
						{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
					}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha2).Return(
					[]github.BasePull{}, nil,
				)
				gh.EXPECT().GetPullRequest(gomock.Any(), "willabides", "semver-next", 14).Return(
					&github.BasePull{
						Number: 14,
						Labels: []string{labelMinor},
					}, nil,
				)
				gh.EXPECT().GetPullRequestCommits(gomock.Any(), "willabides", "semver-next", 14).Return(
					[]string{sha1}, nil,
				)
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        "v0.15.0",
				PrevVersion: "0.15.0",
				Head:        sha1,
				CheckPR:     14,
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("0.16.0"),
				PreviousVersion: *semver.MustParse("0.15.0"),
				ChangeLevel:     changeLevelMinor,
			},
		},
		{
			name: "no change",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{
						AheadBy: 0,
						Commits: []string{sha1, sha2},
					}, nil,
				)
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", mergeSha, sha1, 0).Return(
					&github.CommitComparison{AheadBy: 2}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha1).Return(
					[]github.BasePull{
						{Number: 1, MergeCommitSha: mergeSha, Labels: []string{"something else"}},
						{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelNone}},
						{Number: 3, MergeCommitSha: mergeSha},
						{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelNone}},
					}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha2).Return(
					[]github.BasePull{}, nil,
				)
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        "v0.15.0",
				PrevVersion: "0.15.0",
				Head:        sha1,
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("0.15.0"),
				PreviousVersion: *semver.MustParse("0.15.0"),
				ChangeLevel:     changeLevelNone,
			},
		},
		{
			name: "missing labels",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{
						AheadBy: 0,
						Commits: []string{sha1, sha2},
					}, nil,
				)
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", mergeSha, sha1, 0).Return(
					&github.CommitComparison{AheadBy: 2}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha1).Return(
					[]github.BasePull{
						{Number: 1, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
					}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha2).Return(
					[]github.BasePull{
						{Number: 2, MergeCommitSha: mergeSha, Labels: []string{"something else"}},
						{Number: 3, MergeCommitSha: mergeSha, Labels: []string{}},
					}, nil,
				)
			},
			options: &getNextOptions{
				Repo: "willabides/semver-next",
				Base: "v0.15.0",
				Head: sha1,
			},
			wantErr: "commit 2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa has no labels on associated pull requests: [#2 #3]",
		},
		{
			name: "empty diff",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{AheadBy: 0, Commits: []string{}}, nil,
				)
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        "v0.15.0",
				PrevVersion: "0.15.0",
				Head:        sha1,
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("0.15.0"),
				PreviousVersion: *semver.MustParse("0.15.0"),
				ChangeLevel:     changeLevelNone,
			},
		},
		{
			name: "empty diff ignores minBump",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{AheadBy: 0, Commits: []string{}}, nil,
				)
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        "v0.15.0",
				PrevVersion: "0.15.0",
				Head:        sha1,
				MinBump:     &[]changeLevel{changeLevelPatch}[0],
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("0.15.0"),
				PreviousVersion: *semver.MustParse("0.15.0"),
				ChangeLevel:     changeLevelNone,
			},
		},
		{
			name: "minBump",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{AheadBy: 0, Commits: []string{sha1, sha2}}, nil,
				)
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", mergeSha, sha1, 0).Return(
					&github.CommitComparison{AheadBy: 2}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha1).Return(
					[]github.BasePull{
						{Number: 1, MergeCommitSha: mergeSha, Labels: []string{"something else"}},
						{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
						{Number: 3, MergeCommitSha: mergeSha},
						{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
					}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha2).Return(
					[]github.BasePull{}, nil,
				)
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        "v0.15.0",
				PrevVersion: "0.15.0",
				Head:        sha1,
				MinBump:     &[]changeLevel{changeLevelMinor}[0],
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("0.16.0"),
				PreviousVersion: *semver.MustParse("0.15.0"),
				ChangeLevel:     changeLevelMinor,
			},
		},
		{
			name: "compareCommits error",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					nil, assert.AnError,
				)
			},
			options: &getNextOptions{
				Repo: "willabides/semver-next",
				Base: "v0.15.0",
				Head: sha1,
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name: "listPullRequestsWithCommit error",
			setupMocks: func(gh *MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), "willabides", "semver-next", "v0.15.0", sha1, -1).Return(
					&github.CommitComparison{AheadBy: 0, Commits: []string{sha1, sha2, sha3}}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha1).Return(
					nil, assert.AnError,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha2).Return(
					[]github.BasePull{}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), "willabides", "semver-next", sha3).Return(
					nil, assert.AnError,
				)
			},
			options: &getNextOptions{
				Repo: "willabides/semver-next",
				Base: "v0.15.0",
				Head: sha1,
			},
			wantErr: errors.Join(assert.AnError, assert.AnError).Error(),
		},
		{
			name:       "prev version not valid semver",
			setupMocks: func(gh *MockGithubClient) {},
			options: &getNextOptions{
				PrevVersion: "foo",
			},
			wantErr: `invalid previous version "foo": Invalid Semantic Version`,
		},
		{
			name:       "invalid repo",
			setupMocks: func(gh *MockGithubClient) {},
			options: &getNextOptions{
				Repo:        "foo",
				PrevVersion: "1.2.3",
			},
			wantErr: `repo must be in the form owner/name`,
		},
		{
			name:       "minBump > maxBump",
			setupMocks: func(gh *MockGithubClient) {},
			options: &getNextOptions{
				MinBump: &[]changeLevel{changeLevelMajor}[0],
				MaxBump: &[]changeLevel{changeLevelMinor}[0],
			},
			wantErr: "minBump must be less than or equal to maxBump",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gh := mockGithubClient(t)
			tt.setupMocks(gh)

			// Set the GithubClient in options if not already set
			if tt.options.GithubClient == nil {
				tt.options.GithubClient = gh
			}

			got, err := getNext(ctx, tt.options)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_bumpVersion(t *testing.T) {
	for _, td := range []struct {
		name    string
		prev    string
		minBump changeLevel
		maxBump changeLevel
		commits []gitCommit
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
			prev: "1.2.3",
			commits: []gitCommit{
				{
					Pulls: []ghPull{
						{
							ChangeLevel: changeLevelPatch,
							Number:      1,
						},
						{
							ChangeLevel: changeLevelMinor,
							Number:      2,
						},
					},
				},
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("1.3.0"),
				PreviousVersion: *semver.MustParse("1.2.3"),
				ChangeLevel:     changeLevelMinor,
			},
		},
		{
			name: "new prerelease",
			prev: "1.2.3",
			commits: []gitCommit{
				{
					Pulls: []ghPull{{
						ChangeLevel: changeLevelPatch,
						Number:      1,
						HasPreLabel: true,
					}},
				},
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
				{
					Pulls: []ghPull{{
						ChangeLevel: changeLevelPatch,
						Number:      1,
						HasPreLabel: true,
					}, {
						ChangeLevel: changeLevelNone,
						Number:      2,
						HasPreLabel: true,
					}},
				},
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
				{
					Pulls: []ghPull{{
						ChangeLevel:      changeLevelPatch,
						Number:           1,
						HasPreLabel:      true,
						PreReleasePrefix: "alpha",
					}, {
						ChangeLevel:      changeLevelNone,
						Number:           2,
						HasPreLabel:      true,
						PreReleasePrefix: "beta",
					}},
				},
			},
			wantErr: `cannot have multiple pre-release prefixes in the same release. pre-release prefix. release contains both "alpha" and "beta"`,
		},
		{
			name: "mixed prerelease and non-prerelease on stable",
			prev: "1.2.3",
			commits: []gitCommit{
				{
					Pulls: []ghPull{{
						ChangeLevel: changeLevelPatch,
						Number:      1,
						HasPreLabel: true,
					}, {
						ChangeLevel: changeLevelPatch,
						Number:      2,
						HasPreLabel: false,
					}},
				},
			},
			wantErr: "cannot have pre-release and non-pre-release PRs in the same release. pre-release PRs: [#1], non-pre-release PRs: [#2]",
		},
		{
			name: "mixed prerelease and non-prerelease on prerelease",
			prev: "1.2.3-0",
			commits: []gitCommit{
				{
					Pulls: []ghPull{{
						ChangeLevel: changeLevelPatch,
						Number:      1,
						HasPreLabel: true,
					}, {
						ChangeLevel: changeLevelPatch,
						Number:      2,
						HasPreLabel: false,
					}},
				},
			},
			wantErr: "cannot have pre-release and non-pre-release PRs in the same release. pre-release PRs: [#1], non-pre-release PRs: [#2]",
		},
		{
			name: "stable tag only",
			prev: "0.1.0-0",
			commits: []gitCommit{
				{
					Pulls: []ghPull{
						{
							Number:         1,
							HasStableLabel: true,
						},
					},
				},
			},
			want: &getNextResult{
				NextVersion:     *semver.MustParse("0.1.0"),
				PreviousVersion: *semver.MustParse("0.1.0-0"),
				ChangeLevel:     changeLevelNone,
			},
		},
	} {
		t.Run(td.name, func(t *testing.T) {
			prev := semver.MustParse(td.prev)
			got, err := bumpVersion(context.Background(), *prev, td.minBump, td.maxBump, td.commits)
			if td.wantErr != "" {
				require.EqualError(t, err, td.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, td.want, got)
		})
	}
}
