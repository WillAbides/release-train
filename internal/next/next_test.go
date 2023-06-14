package next

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train-action/v2/internal"
	"github.com/willabides/release-train-action/v2/internal/testutil"
)

type listPullRequestsWithCommitCall struct {
	owner, repo, sha string
	result           []internal.Pull
	err              error
}

func mockListPullRequestsWithCommit(t *testing.T, calls []listPullRequestsWithCommitCall) func(ctx context.Context, owner, repo, sha string) ([]internal.Pull, error) {
	var lock sync.Mutex
	return func(ctx context.Context, owner, repo, sha string) ([]internal.Pull, error) {
		lock.Lock()
		defer lock.Unlock()
		idx := 0
		for ; idx < len(calls); idx++ {
			if calls[idx].owner == owner && calls[idx].repo == repo && calls[idx].sha == sha {
				break
			}
		}
		if !assert.Less(t, idx, len(calls), "unexpected call to ListPullRequestsWithCommit") {
			return nil, fmt.Errorf("unexpected call to ListPullRequestsWithCommit")
		}
		call := calls[idx]
		calls = append(calls[:idx], calls[idx+1:]...)
		return call.result, call.err
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
			StubListPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []internal.Pull{
						// non-standard caps to test case insensitivity
						{Number: 1, Labels: []string{strings.ToUpper(internal.ChangeLevelMajor.String()), "something else"}},
						{Number: 2, Labels: []string{"something else"}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelMinor.String()}},
					},
				},
				{
					owner:  "willabides",
					repo:   "semver-next",
					sha:    sha2,
					result: []internal.Pull{},
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
						{Number: 1, Labels: []string{"major"}, ChangeLevel: internal.ChangeLevelMajor},
						{Number: 2, Labels: []string{}},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{"minor"}, ChangeLevel: internal.ChangeLevelMinor},
					},
					ChangeLevel: internal.ChangeLevelMajor,
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
			StubListPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []internal.Pull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{internal.ChangeLevelMinor.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelPatch.String()}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []internal.Pull{},
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
						{Number: 1, Labels: []string{}},
						{Number: 2, Labels: []string{"minor"}, ChangeLevel: internal.ChangeLevelMinor},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{"patch"}, ChangeLevel: internal.ChangeLevelPatch},
					},
					ChangeLevel: internal.ChangeLevelMinor,
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
			StubListPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []internal.Pull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{internal.ChangeLevelPatch.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelPatch.String()}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []internal.Pull{},
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
						{Number: 1, Labels: []string{}},
						{Number: 2, Labels: []string{internal.ChangeLevelPatch.String()}, ChangeLevel: internal.ChangeLevelPatch},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{internal.ChangeLevelPatch.String()}, ChangeLevel: internal.ChangeLevelPatch},
					},
					ChangeLevel: internal.ChangeLevelPatch,
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
			StubListPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []internal.Pull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{internal.ChangeLevelNoChange.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelNoChange.String()}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []internal.Pull{},
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
						{Number: 1, Labels: []string{}},
						{Number: 2, Labels: []string{internal.ChangeLevelNoChange.String()}, ChangeLevel: internal.ChangeLevelNoChange},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{internal.ChangeLevelNoChange.String()}, ChangeLevel: internal.ChangeLevelNoChange},
					},
					ChangeLevel: internal.ChangeLevelNoChange,
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
			StubListPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []internal.Pull{
						{Number: 1, Labels: []string{"patch"}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []internal.Pull{
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
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("%s (#2, #3)", sha2))
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
			StubListPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []internal.Pull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{internal.ChangeLevelPatch.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{internal.ChangeLevelPatch.String()}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []internal.Pull{},
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
						{Number: 1, Labels: []string{}},
						{Number: 2, Labels: []string{internal.ChangeLevelPatch.String()}, ChangeLevel: internal.ChangeLevelPatch},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{internal.ChangeLevelPatch.String()}, ChangeLevel: internal.ChangeLevelPatch},
					},
					ChangeLevel: internal.ChangeLevelPatch,
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
			StubListPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{owner: "willabides", repo: "semver-next", sha: sha1, err: assert.AnError},
				{owner: "willabides", repo: "semver-next", sha: sha2, result: []internal.Pull{}},
				{owner: "willabides", repo: "semver-next", sha: sha3, err: assert.AnError},
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
