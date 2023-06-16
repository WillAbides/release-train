package next

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train-action/v2/internal"
	"github.com/willabides/release-train-action/v2/internal/testutil"
)

func Test_incrPre(t *testing.T) {
	for _, td := range []struct {
		prev    string
		level   internal.ChangeLevel
		prefix  string
		want    string
		wantErr string
	}{
		{
			prev:  "1.2.3",
			level: internal.ChangeLevelMajor,
			want:  "2.0.0-0",
		},
		{
			prev:  "1.0.0-alpha.0",
			level: internal.ChangeLevelMinor,
			want:  "1.0.0-alpha.1",
		},
		{
			prev:  "1.0.0-0",
			level: internal.ChangeLevelMinor,
			want:  "1.0.0-1",
		},
		{
			prev:  "1.0.1-0",
			level: internal.ChangeLevelMinor,
			want:  "1.1.0-0",
		},
		{
			prev:  "1.0.1-0",
			level: internal.ChangeLevelPatch,
			want:  "1.0.1-1",
		},
		{
			prev:  "1.0.1-0",
			level: internal.ChangeLevelMajor,
			want:  "2.0.0-0",
		},
		{
			prev:   "1.0.1-0",
			level:  internal.ChangeLevelMajor,
			prefix: "alpha",
			want:   "2.0.0-alpha.0",
		},
		{
			prev:    "1.2.3",
			level:   internal.ChangeLevelNoChange,
			prefix:  "alpha",
			wantErr: `invalid change level for pre-release: no change`,
		},
		{
			prev:    "1.2.3-beta.0",
			level:   internal.ChangeLevelPatch,
			prefix:  "alpha",
			wantErr: `pre-release version "1.2.3-alpha.0" is not greater than "1.2.3-beta.0"`,
		},
		{
			prev:   "1.2.3-beta.0",
			level:  internal.ChangeLevelPatch,
			prefix: "",
			want:   "1.2.3-beta.1",
		},
		{
			prev:    "1.2.3-beta.0",
			level:   internal.ChangeLevelPatch,
			prefix:  "_invalid",
			wantErr: "Invalid Prerelease string",
		},
		{
			prev:   "1.2.3-rc0",
			level:  internal.ChangeLevelPatch,
			prefix: "",
			want:   "1.2.3-rc0.0",
		},
		{
			prev:    "1.2.3-rc0",
			level:   internal.ChangeLevelPatch,
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

func Test_next(t *testing.T) {
	ctx := context.Background()

	sha1 := "1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha2 := "2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sha3 := "3aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	t.Run("major", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0", sha1}, []string{owner, repo, base, head})
				return []string{sha1, sha2}, nil
			},
			StubListPullRequestsWithCommit: testutil.MockListPullRequestsWithCommit(t, []testutil.ListPullRequestsWithCommitCall{
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha1,
					Result: []internal.BasePull{
						// non-standard caps to test case insensitivity
						{Number: 1, Labels: []string{strings.ToUpper(internal.ChangeLevelMajor.String()), "something else"}},
						{Number: 2, Labels: []string{"something else"}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelMinor.String()}},
					},
				},
				{
					Owner:  "willabides",
					Repo:   "semver-next",
					Sha:    sha2,
					Result: []internal.BasePull{},
				},
			}),
		}
		got, err := GetNext(
			ctx,
			&Options{
				Repo:         "willabides/semver-next",
				Base:         "v0.15.0",
				Head:         sha1,
				GithubClient: &gh,
			},
		)
		require.NoError(t, err)
		want := Result{
			NextVersion:     "1.0.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     internal.ChangeLevelMajor,
			Commits: []Commit{
				{
					Sha: sha1,
					Pulls: []internal.Pull{
						{Number: 1, LevelLabels: []string{"MAJOR"}, ChangeLevel: internal.ChangeLevelMajor},
						{Number: 2},
						{Number: 3},
						{Number: 4, LevelLabels: []string{"minor"}, ChangeLevel: internal.ChangeLevelMinor},
					},
				},
				{
					Sha:   sha2,
					Pulls: []internal.Pull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("minor", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			StubListPullRequestsWithCommit: testutil.MockListPullRequestsWithCommit(t, []testutil.ListPullRequestsWithCommitCall{
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha1,
					Result: []internal.BasePull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{internal.ChangeLevelMinor.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelPatch.String()}},
					},
				},
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha2,
					Result: []internal.BasePull{},
				},
			}),
		}
		got, err := GetNext(
			ctx,
			&Options{
				Repo:         "willabides/semver-next",
				Base:         "v0.15.0",
				Head:         sha1,
				GithubClient: &gh,
			},
		)
		require.NoError(t, err)
		want := Result{
			NextVersion:     "0.16.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     internal.ChangeLevelMinor,
			Commits: []Commit{
				{
					Sha: sha1,
					Pulls: []internal.Pull{
						{Number: 1},
						{Number: 2, LevelLabels: []string{"minor"}, ChangeLevel: internal.ChangeLevelMinor},
						{Number: 3},
						{Number: 4, LevelLabels: []string{"patch"}, ChangeLevel: internal.ChangeLevelPatch},
					},
				},
				{
					Sha:   sha2,
					Pulls: []internal.Pull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("patch", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			StubListPullRequestsWithCommit: testutil.MockListPullRequestsWithCommit(t, []testutil.ListPullRequestsWithCommitCall{
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha1,
					Result: []internal.BasePull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{internal.ChangeLevelPatch.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelPatch.String()}},
					},
				},
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha2,
					Result: []internal.BasePull{},
				},
			}),
		}
		got, err := GetNext(
			ctx,
			&Options{
				Repo:         "willabides/semver-next",
				Base:         "v0.15.0",
				Head:         sha1,
				GithubClient: &gh,
			},
		)
		require.NoError(t, err)
		want := Result{
			NextVersion:     "0.15.1",
			PreviousVersion: "0.15.0",
			ChangeLevel:     internal.ChangeLevelPatch,
			Commits: []Commit{
				{
					Sha: sha1,
					Pulls: []internal.Pull{
						{Number: 1},
						{Number: 2, LevelLabels: []string{internal.ChangeLevelPatch.String()}, ChangeLevel: internal.ChangeLevelPatch},
						{Number: 3},
						{Number: 4, LevelLabels: []string{internal.ChangeLevelPatch.String()}, ChangeLevel: internal.ChangeLevelPatch},
					},
				},
				{
					Sha:   sha2,
					Pulls: []internal.Pull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("no change", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			StubListPullRequestsWithCommit: testutil.MockListPullRequestsWithCommit(t, []testutil.ListPullRequestsWithCommitCall{
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha1,
					Result: []internal.BasePull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{internal.ChangeLevelNoChange.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelNoChange.String()}},
					},
				},
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha2,
					Result: []internal.BasePull{},
				},
			}),
		}
		got, err := GetNext(
			ctx,
			&Options{
				Repo:         "willabides/semver-next",
				Base:         "v0.15.0",
				Head:         sha1,
				GithubClient: &gh,
			},
		)
		require.NoError(t, err)
		want := Result{
			NextVersion:     "0.15.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     internal.ChangeLevelNoChange,
			Commits: []Commit{
				{
					Sha: sha1,
					Pulls: []internal.Pull{
						{Number: 1},
						{Number: 2, LevelLabels: []string{internal.ChangeLevelNoChange.String()}, ChangeLevel: internal.ChangeLevelNoChange},
						{Number: 3},
						{Number: 4, LevelLabels: []string{internal.ChangeLevelNoChange.String()}, ChangeLevel: internal.ChangeLevelNoChange},
					},
				},
				{
					Sha:   sha2,
					Pulls: []internal.Pull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("missing labels", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			StubListPullRequestsWithCommit: testutil.MockListPullRequestsWithCommit(t, []testutil.ListPullRequestsWithCommitCall{
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha1,
					Result: []internal.BasePull{
						{Number: 1, Labels: []string{"patch"}},
					},
				},
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha2,
					Result: []internal.BasePull{
						{Number: 2, Labels: []string{"something else"}},
						{Number: 3, Labels: []string{}},
					},
				},
			}),
		}
		_, err := GetNext(ctx, &Options{
			Repo:         "willabides/semver-next",
			Base:         "v0.15.0",
			Head:         sha1,
			GithubClient: &gh,
		})
		require.EqualError(t, err, "commit 2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa has no labels on associated pull requests: [#2 #3]")
	})

	t.Run("empty diff", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{}, nil
			},
		}
		got, err := GetNext(ctx, &Options{
			Repo:         "willabides/semver-next",
			Base:         "v0.15.0",
			Head:         sha1,
			GithubClient: &gh,
		})
		require.NoError(t, err)
		want := Result{
			NextVersion:     "0.15.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     internal.ChangeLevelNoChange,
			Commits:         []Commit{},
		}
		require.Equal(t, &want, got)
	})

	t.Run("empty diff ignores minBump", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{}, nil
			},
		}
		got, err := GetNext(ctx, &Options{
			Repo:         "willabides/semver-next",
			Base:         "v0.15.0",
			Head:         sha1,
			MinBump:      internal.ChangeLevelPatch.String(),
			GithubClient: &gh,
		})
		require.NoError(t, err)
		want := Result{
			NextVersion:     "0.15.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     internal.ChangeLevelNoChange,
			Commits:         []Commit{},
		}
		require.Equal(t, &want, got)
	})

	t.Run("minBump", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			StubListPullRequestsWithCommit: testutil.MockListPullRequestsWithCommit(t, []testutil.ListPullRequestsWithCommitCall{
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha1,
					Result: []internal.BasePull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{internal.ChangeLevelPatch.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelPatch.String()}},
					},
				},
				{
					Owner: "willabides", Repo: "semver-next", Sha: sha2,
					Result: []internal.BasePull{},
				},
			}),
		}
		got, err := GetNext(
			ctx,
			&Options{
				Repo:         "willabides/semver-next",
				Base:         "v0.15.0",
				Head:         sha1,
				MinBump:      internal.ChangeLevelMinor.String(),
				GithubClient: &gh,
			},
		)
		require.NoError(t, err)
		want := Result{
			NextVersion:     "0.16.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     internal.ChangeLevelMinor,
			Commits: []Commit{
				{
					Sha: sha1,
					Pulls: []internal.Pull{
						{Number: 1},
						{Number: 2, LevelLabels: []string{internal.ChangeLevelPatch.String()}, ChangeLevel: internal.ChangeLevelPatch},
						{Number: 3},
						{Number: 4, LevelLabels: []string{internal.ChangeLevelPatch.String()}, ChangeLevel: internal.ChangeLevelPatch},
					},
				},
				{
					Sha:   sha2,
					Pulls: []internal.Pull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("compareCommits error", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return nil, assert.AnError
			},
		}
		_, err := GetNext(ctx, &Options{
			Repo:         "willabides/semver-next",
			Base:         "v0.15.0",
			Head:         sha1,
			GithubClient: &gh,
		})
		require.EqualError(t, err, assert.AnError.Error())
	})

	t.Run("listPullRequestsWithCommit error", func(t *testing.T) {
		gh := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2, sha3}, nil
			},
			StubListPullRequestsWithCommit: testutil.MockListPullRequestsWithCommit(t, []testutil.ListPullRequestsWithCommitCall{
				{Owner: "willabides", Repo: "semver-next", Sha: sha1, Err: assert.AnError},
				{Owner: "willabides", Repo: "semver-next", Sha: sha2, Result: []internal.BasePull{}},
				{Owner: "willabides", Repo: "semver-next", Sha: sha3, Err: assert.AnError},
			}),
		}
		_, err := GetNext(ctx, &Options{
			Repo:         "willabides/semver-next",
			Base:         "v0.15.0",
			Head:         sha1,
			GithubClient: &gh,
		})
		require.EqualError(t, err, errors.Join(assert.AnError, assert.AnError).Error())
	})

	t.Run("invalid minBump", func(t *testing.T) {
		_, err := GetNext(ctx, &Options{MinBump: "foo"})
		require.EqualError(t, err, "invalid change level: foo")
	})

	t.Run("invalid maxBump", func(t *testing.T) {
		_, err := GetNext(ctx, &Options{MaxBump: "foo"})
		require.EqualError(t, err, "invalid change level: foo")
	})

	t.Run("prev version not valid semver", func(t *testing.T) {
		_, err := GetNext(ctx, &Options{PrevVersion: "foo"})
		require.EqualError(t, err, `invalid previous version "foo": Invalid Semantic Version`)
	})

	t.Run("invalid repo", func(t *testing.T) {
		_, err := GetNext(ctx, &Options{Repo: "foo", PrevVersion: "1.2.3"})
		require.EqualError(t, err, `repo must be in the form owner/name`)
	})

	t.Run("minBump > maxBump", func(t *testing.T) {
		_, err := GetNext(ctx, &Options{MinBump: "major", MaxBump: "minor"})
		require.EqualError(t, err, "minBump must be less than or equal to maxBump")
	})
}

func Test_bumpVersion(t *testing.T) {
	for _, td := range []struct {
		name    string
		prev    string
		minBump internal.ChangeLevel
		maxBump internal.ChangeLevel
		commits []Commit
		want    *Result
		wantErr string
	}{
		{
			name: "no commits",
			prev: "1.2.3",
			want: &Result{
				NextVersion:     "1.2.3",
				PreviousVersion: "1.2.3",
			},
		},
		{
			name: "no commits, prerelease",
			prev: "1.2.3-alpha.0",
			want: &Result{
				NextVersion:     "1.2.3-alpha.0",
				PreviousVersion: "1.2.3-alpha.0",
			},
		},
		{
			name: "bump stable",
			prev: "1.2.3",
			commits: []Commit{
				{
					Pulls: []internal.Pull{
						{
							ChangeLevel: internal.ChangeLevelPatch,
							Number:      1,
						},
						{
							ChangeLevel: internal.ChangeLevelMinor,
							Number:      2,
						},
					},
				},
			},
			want: &Result{
				NextVersion:     "1.3.0",
				PreviousVersion: "1.2.3",
				ChangeLevel:     internal.ChangeLevelMinor,
			},
		},
		{
			name: "new prerelease",
			prev: "1.2.3",
			commits: []Commit{
				{
					Pulls: []internal.Pull{{
						ChangeLevel: internal.ChangeLevelPatch,
						Number:      1,
						HasPreLabel: true,
					}},
				},
			},
			want: &Result{
				NextVersion:     "1.2.4-0",
				PreviousVersion: "1.2.3",
				ChangeLevel:     internal.ChangeLevelPatch,
			},
		},
		{
			name: "bump prerelease using previous prefix",
			prev: "1.2.3-alpha.33",
			commits: []Commit{
				{
					Pulls: []internal.Pull{{
						ChangeLevel: internal.ChangeLevelPatch,
						Number:      1,
						HasPreLabel: true,
					}, {
						ChangeLevel: internal.ChangeLevelNoChange,
						Number:      2,
						HasPreLabel: true,
					}},
				},
			},
			want: &Result{
				NextVersion:     "1.2.3-alpha.34",
				PreviousVersion: "1.2.3-alpha.33",
				ChangeLevel:     internal.ChangeLevelPatch,
			},
		},
		{
			name: "mixed prefixes",
			prev: "1.2.3",
			commits: []Commit{
				{
					Pulls: []internal.Pull{{
						ChangeLevel:      internal.ChangeLevelPatch,
						Number:           1,
						HasPreLabel:      true,
						PreReleasePrefix: "alpha",
					}, {
						ChangeLevel:      internal.ChangeLevelNoChange,
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
			commits: []Commit{
				{
					Pulls: []internal.Pull{{
						ChangeLevel: internal.ChangeLevelPatch,
						Number:      1,
						HasPreLabel: true,
					}, {
						ChangeLevel: internal.ChangeLevelNoChange,
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
			commits: []Commit{
				{
					Pulls: []internal.Pull{{
						ChangeLevel: internal.ChangeLevelPatch,
						Number:      1,
						HasPreLabel: true,
					}, {
						ChangeLevel: internal.ChangeLevelNoChange,
						Number:      2,
						HasPreLabel: false,
					}},
				},
			},
			wantErr: "cannot have pre-release and non-pre-release PRs in the same release. pre-release PRs: [#1], non-pre-release PRs: [#2]",
		},
	} {
		t.Run(td.name, func(t *testing.T) {
			prev := semver.MustParse(td.prev)
			got, err := bumpVersion(*prev, td.minBump, td.maxBump, td.commits)
			if td.wantErr != "" {
				require.EqualError(t, err, td.wantErr)
				return
			}
			if got != nil {
				got.Commits = nil
			}
			require.NoError(t, err)
			require.Equal(t, td.want, got)
		})
	}
}
