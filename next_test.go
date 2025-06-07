package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train/v3/internal/github"
	"github.com/willabides/release-train/v3/internal/mocks"
	"go.uber.org/mock/gomock"
)

func TestGetNext(t *testing.T) {
	ctx := context.Background()

	sha1 := "1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha3 := "3aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mergeSha := "4aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	baseTag := "v0.15.0"
	miscLabel := "something else"
	repoOwner := "willabides"
	repo := "semver-next"

	cmpBaseTagToSha1 := &github.CommitComparison{AheadBy: 2, Commits: []string{sha1, sha2}}
	cmpMergeShaToSha1 := &github.CommitComparison{AheadBy: 2}

	tests := []struct {
		name       string
		setupMocks func(*mocks.MockGithubClient)
		options    *getNextOptions
		want       *getNextResult
		wantErr    string

		noStubs              bool
		cmpBaseTagToSha1     *github.CommitComparison
		cmpMergeShaToSha1    *github.CommitComparison
		sha1MergedPulls      []github.BasePull
		sha2MergedPulls      []github.BasePull
		pullRequest14        *github.BasePull
		pullRequest14Commits []string
	}{
		{
			name: "major",
			sha1MergedPulls: []github.BasePull{
				// non-standard caps to test case insensitivity
				{Number: 1, MergeCommitSha: mergeSha, Labels: []string{"SEMVER:BREAKING", miscLabel}},
				{Number: 2, MergeCommitSha: mergeSha, Labels: []string{miscLabel}},
				{Number: 3, MergeCommitSha: mergeSha},
				{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelMinor}},
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        baseTag,
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
			sha1MergedPulls: []github.BasePull{
				{Number: 1, MergeCommitSha: mergeSha, Labels: []string{miscLabel}},
				{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelMinor}},
				{Number: 3, MergeCommitSha: mergeSha},
				{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        baseTag,
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
			sha1MergedPulls: []github.BasePull{
				{Number: 1, MergeCommitSha: mergeSha, Labels: []string{miscLabel}},
				{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
				{Number: 3, MergeCommitSha: mergeSha},
				{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        baseTag,
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
			sha1MergedPulls: []github.BasePull{
				{Number: 1, MergeCommitSha: mergeSha, Labels: []string{miscLabel}},
				{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
				{Number: 3, MergeCommitSha: mergeSha},
				{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
			},
			pullRequest14: &github.BasePull{
				Number: 14,
				Labels: []string{labelMinor},
			},
			pullRequest14Commits: []string{sha1},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        baseTag,
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
			name:             "no change",
			cmpBaseTagToSha1: &github.CommitComparison{AheadBy: 0, Commits: []string{sha1, sha2}},
			sha1MergedPulls: []github.BasePull{
				{Number: 1, MergeCommitSha: mergeSha, Labels: []string{miscLabel}},
				{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelNone}},
				{Number: 3, MergeCommitSha: mergeSha},
				{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelNone}},
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        baseTag,
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
			name:             "missing labels",
			cmpBaseTagToSha1: &github.CommitComparison{AheadBy: 0, Commits: []string{sha1, sha2}},
			sha1MergedPulls: []github.BasePull{
				{Number: 1, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
			},
			sha2MergedPulls: []github.BasePull{
				{Number: 2, MergeCommitSha: mergeSha, Labels: []string{miscLabel}},
				{Number: 3, MergeCommitSha: mergeSha, Labels: []string{}},
			},
			options: &getNextOptions{
				Repo: "willabides/semver-next",
				Base: baseTag,
				Head: sha1,
			},
			wantErr: "commit 2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa has no labels on associated pull requests: [#2 #3]",
		},
		{
			name:             "empty diff",
			cmpBaseTagToSha1: &github.CommitComparison{AheadBy: 0, Commits: []string{}},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        baseTag,
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
			name:             "empty diff ignores minBump",
			cmpBaseTagToSha1: &github.CommitComparison{AheadBy: 0, Commits: []string{}},

			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        baseTag,
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
			name:             "minBump",
			cmpBaseTagToSha1: &github.CommitComparison{AheadBy: 0, Commits: []string{sha1, sha2}},
			sha1MergedPulls: []github.BasePull{
				{Number: 1, MergeCommitSha: mergeSha, Labels: []string{miscLabel}},
				{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
				{Number: 3, MergeCommitSha: mergeSha},
				{Number: 4, MergeCommitSha: mergeSha, Labels: []string{labelPatch}},
			},
			options: &getNextOptions{
				Repo:        "willabides/semver-next",
				Base:        baseTag,
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
			name:    "compareCommits error",
			noStubs: true,
			setupMocks: func(gh *mocks.MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), repoOwner, repo, baseTag, sha1, -1).Return(
					nil, assert.AnError,
				)
			},
			options: &getNextOptions{
				Repo: "willabides/semver-next",
				Base: baseTag,
				Head: sha1,
			},
			wantErr: assert.AnError.Error(),
		},
		{
			name:    "listPullRequestsWithCommit error",
			noStubs: true,
			setupMocks: func(gh *mocks.MockGithubClient) {
				gh.EXPECT().CompareCommits(gomock.Any(), repoOwner, repo, baseTag, sha1, -1).Return(
					&github.CommitComparison{AheadBy: 0, Commits: []string{sha1, sha2, sha3}}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), repoOwner, repo, sha1).Return(
					nil, assert.AnError,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), repoOwner, repo, sha2).Return(
					[]github.BasePull{}, nil,
				)
				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), repoOwner, repo, sha3).Return(
					nil, assert.AnError,
				)
			},
			options: &getNextOptions{
				Repo: "willabides/semver-next",
				Base: baseTag,
				Head: sha1,
			},
			wantErr: errors.Join(assert.AnError, assert.AnError).Error(),
		},
		{
			name: "prev version not valid semver",
			options: &getNextOptions{
				PrevVersion: "foo",
			},
			wantErr: `invalid previous version "foo": Invalid Semantic Version`,
		},
		{
			name: "invalid repo",
			options: &getNextOptions{
				Repo:        "foo",
				PrevVersion: "1.2.3",
			},
			wantErr: `repo must be in the form owner/name`,
		},
		{
			name: "minBump > maxBump",
			options: &getNextOptions{
				MinBump: &[]changeLevel{changeLevelMajor}[0],
				MaxBump: &[]changeLevel{changeLevelMinor}[0],
			},
			wantErr: "minBump must be less than or equal to maxBump",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gh := mocks.NewMockGithubClient(gomock.NewController(t))

			if !tt.noStubs {
				gh.EXPECT().CompareCommits(gomock.Any(), repoOwner, repo, baseTag, gomock.Any(), -1).Return(
					ptrOr(tt.cmpBaseTagToSha1, cmpBaseTagToSha1), nil,
				).AnyTimes()

				gh.EXPECT().CompareCommits(gomock.Any(), repoOwner, repo, mergeSha, sha1, 0).Return(
					ptrOr(tt.cmpMergeShaToSha1, cmpMergeShaToSha1), nil,
				).AnyTimes()

				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), repoOwner, repo, sha1).Return(
					tt.sha1MergedPulls, nil,
				).AnyTimes()

				gh.EXPECT().ListMergedPullsForCommit(gomock.Any(), repoOwner, repo, sha2).Return(
					tt.sha2MergedPulls, nil,
				).AnyTimes()

				gh.EXPECT().GetPullRequest(gomock.Any(), repoOwner, repo, 14).Return(
					tt.pullRequest14, nil,
				).AnyTimes()

				gh.EXPECT().GetPullRequestCommits(gomock.Any(), repoOwner, repo, 14).Return(
					tt.pullRequest14Commits, nil,
				).AnyTimes()
			}

			if tt.setupMocks != nil {
				tt.setupMocks(gh)
			}

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

// like cmp.Or but for nilable pointers
func ptrOr[T any](pointers ...*T) *T {
	for _, p := range pointers {
		if p != nil {
			return p
		}
	}
	return nil
}
