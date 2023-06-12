package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/v53/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wrapperStub struct {
	listPullRequestsWithCommit func(ctx context.Context, owner, repo, sha string) ([]nextResultPull, error)
	compareCommits             func(ctx context.Context, owner, repo, base, head string) ([]string, error)
	generateReleaseNotes       func(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error)
	createRelease              func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) error
}

var _ wrapper = &wrapperStub{}

func (w *wrapperStub) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]nextResultPull, error) {
	return w.listPullRequestsWithCommit(ctx, owner, repo, sha)
}

func (w *wrapperStub) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error) {
	return w.compareCommits(ctx, owner, repo, base, head)
}

func (w *wrapperStub) GenerateReleaseNotes(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
	return w.generateReleaseNotes(ctx, owner, repo, opts)
}

func (w *wrapperStub) CreateRelease(ctx context.Context, owner, repo string, release *github.RepositoryRelease) error {
	return w.createRelease(ctx, owner, repo, release)
}

type listPullRequestsWithCommitCall struct {
	owner, repo, sha string
	result           []nextResultPull
	err              error
}

func mockListPullRequestsWithCommit(t *testing.T, calls []listPullRequestsWithCommitCall) func(ctx context.Context, owner, repo, sha string) ([]nextResultPull, error) {
	var lock sync.Mutex
	return func(ctx context.Context, owner, repo, sha string) ([]nextResultPull, error) {
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
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0", sha1}, []string{owner, repo, base, head})
				return []string{sha1, sha2}, nil
			},
			listPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []nextResultPull{
						// non-standard caps to test case insensitivity
						{Number: 1, Labels: []string{strings.ToUpper(changeLevelMajor.String()), "something else"}},
						{Number: 2, Labels: []string{"something else"}},
						{Number: 3},
						{Number: 4, Labels: []string{changeLevelMinor.String()}},
					},
				},
				{
					owner:  "willabides",
					repo:   "semver-next",
					sha:    sha2,
					result: []nextResultPull{},
				},
			}),
		}
		got, err := getNext(
			ctx,
			&nextOptions{
				repo: "willabides/semver-next",
				base: "v0.15.0",
				head: sha1,
				gh:   &gh,
			},
		)
		require.NoError(t, err)
		want := nextResult{
			NextVersion:     "1.0.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     changeLevelMajor,
			Commits: []nextResultCommit{
				{
					Sha: sha1,
					Pulls: []nextResultPull{
						{Number: 1, Labels: []string{"major"}, ChangeLevel: changeLevelMajor},
						{Number: 2, Labels: []string{}},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{"minor"}, ChangeLevel: changeLevelMinor},
					},
					ChangeLevel: changeLevelMajor,
				},
				{
					Sha:   sha2,
					Pulls: []nextResultPull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("minor", func(t *testing.T) {
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			listPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []nextResultPull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{changeLevelMinor.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{changeLevelPatch.String()}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []nextResultPull{},
				},
			}),
		}
		got, err := getNext(
			ctx,
			&nextOptions{
				repo: "willabides/semver-next",
				base: "v0.15.0",
				head: sha1,
				gh:   &gh,
			},
		)
		require.NoError(t, err)
		want := nextResult{
			NextVersion:     "0.16.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     changeLevelMinor,
			Commits: []nextResultCommit{
				{
					Sha: sha1,
					Pulls: []nextResultPull{
						{Number: 1, Labels: []string{}},
						{Number: 2, Labels: []string{"minor"}, ChangeLevel: changeLevelMinor},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{"patch"}, ChangeLevel: changeLevelPatch},
					},
					ChangeLevel: changeLevelMinor,
				},
				{
					Sha:   sha2,
					Pulls: []nextResultPull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("patch", func(t *testing.T) {
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			listPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []nextResultPull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{changeLevelPatch.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{changeLevelPatch.String()}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []nextResultPull{},
				},
			}),
		}
		got, err := getNext(
			ctx,
			&nextOptions{
				repo: "willabides/semver-next",
				base: "v0.15.0",
				head: sha1,
				gh:   &gh,
			},
		)
		require.NoError(t, err)
		want := nextResult{
			NextVersion:     "0.15.1",
			PreviousVersion: "0.15.0",
			ChangeLevel:     changeLevelPatch,
			Commits: []nextResultCommit{
				{
					Sha: sha1,
					Pulls: []nextResultPull{
						{Number: 1, Labels: []string{}},
						{Number: 2, Labels: []string{changeLevelPatch.String()}, ChangeLevel: changeLevelPatch},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{changeLevelPatch.String()}, ChangeLevel: changeLevelPatch},
					},
					ChangeLevel: changeLevelPatch,
				},
				{
					Sha:   sha2,
					Pulls: []nextResultPull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("no change", func(t *testing.T) {
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			listPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []nextResultPull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{changeLevelNoChange.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{changeLevelNoChange.String()}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []nextResultPull{},
				},
			}),
		}
		got, err := getNext(
			ctx,
			&nextOptions{
				repo: "willabides/semver-next",
				base: "v0.15.0",
				head: sha1,
				gh:   &gh,
			},
		)
		require.NoError(t, err)
		want := nextResult{
			NextVersion:     "0.15.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     changeLevelNoChange,
			Commits: []nextResultCommit{
				{
					Sha: sha1,
					Pulls: []nextResultPull{
						{Number: 1, Labels: []string{}},
						{Number: 2, Labels: []string{changeLevelNoChange.String()}, ChangeLevel: changeLevelNoChange},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{changeLevelNoChange.String()}, ChangeLevel: changeLevelNoChange},
					},
					ChangeLevel: changeLevelNoChange,
				},
				{
					Sha:   sha2,
					Pulls: []nextResultPull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("missing labels", func(t *testing.T) {
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			listPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []nextResultPull{
						{Number: 1, Labels: []string{"patch"}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []nextResultPull{
						{Number: 2, Labels: []string{"something else"}},
						{Number: 3, Labels: []string{}},
					},
				},
			}),
		}
		_, err := getNext(ctx, &nextOptions{
			repo: "willabides/semver-next",
			base: "v0.15.0",
			head: sha1,
			gh:   &gh,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("%s (#2, #3)", sha2))
	})

	t.Run("empty diff", func(t *testing.T) {
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{}, nil
			},
		}
		got, err := getNext(ctx, &nextOptions{
			repo: "willabides/semver-next",
			base: "v0.15.0",
			head: sha1,
			gh:   &gh,
		})
		require.NoError(t, err)
		want := nextResult{
			NextVersion:     "0.15.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     changeLevelNoChange,
			Commits:         []nextResultCommit{},
		}
		require.Equal(t, &want, got)
	})

	t.Run("empty diff ignores minBump", func(t *testing.T) {
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{}, nil
			},
		}
		got, err := getNext(ctx, &nextOptions{
			repo:    "willabides/semver-next",
			base:    "v0.15.0",
			head:    sha1,
			minBump: changeLevelPatch.String(),
			gh:      &gh,
		})
		require.NoError(t, err)
		want := nextResult{
			NextVersion:     "0.15.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     changeLevelNoChange,
			Commits:         []nextResultCommit{},
		}
		require.Equal(t, &want, got)
	})

	t.Run("minBump", func(t *testing.T) {
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2}, nil
			},
			listPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{
					owner: "willabides", repo: "semver-next", sha: sha1,
					result: []nextResultPull{
						{Number: 1, Labels: []string{"something else"}},
						{Number: 2, Labels: []string{changeLevelPatch.String()}},
						{Number: 3},
						{Number: 4, Labels: []string{changeLevelPatch.String()}},
					},
				},
				{
					owner: "willabides", repo: "semver-next", sha: sha2,
					result: []nextResultPull{},
				},
			}),
		}
		got, err := getNext(
			ctx,
			&nextOptions{
				repo:    "willabides/semver-next",
				base:    "v0.15.0",
				head:    sha1,
				minBump: changeLevelMinor.String(),
				gh:      &gh,
			},
		)
		require.NoError(t, err)
		want := nextResult{
			NextVersion:     "0.16.0",
			PreviousVersion: "0.15.0",
			ChangeLevel:     changeLevelMinor,
			Commits: []nextResultCommit{
				{
					Sha: sha1,
					Pulls: []nextResultPull{
						{Number: 1, Labels: []string{}},
						{Number: 2, Labels: []string{changeLevelPatch.String()}, ChangeLevel: changeLevelPatch},
						{Number: 3, Labels: []string{}},
						{Number: 4, Labels: []string{changeLevelPatch.String()}, ChangeLevel: changeLevelPatch},
					},
					ChangeLevel: changeLevelPatch,
				},
				{
					Sha:   sha2,
					Pulls: []nextResultPull{},
				},
			},
		}
		require.Equal(t, &want, got)
	})

	t.Run("compareCommits error", func(t *testing.T) {
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return nil, assert.AnError
			},
		}
		_, err := getNext(ctx, &nextOptions{
			repo: "willabides/semver-next",
			base: "v0.15.0",
			head: sha1,
			gh:   &gh,
		})
		require.EqualError(t, err, assert.AnError.Error())
	})

	t.Run("listPullRequestsWithCommit error", func(t *testing.T) {
		gh := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, []string{"willabides", "semver-next", "v0.15.0"}, []string{owner, repo, base})
				assert.Equal(t, sha1, head)
				return []string{sha1, sha2, sha3}, nil
			},
			listPullRequestsWithCommit: mockListPullRequestsWithCommit(t, []listPullRequestsWithCommitCall{
				{owner: "willabides", repo: "semver-next", sha: sha1, err: assert.AnError},
				{owner: "willabides", repo: "semver-next", sha: sha2, result: []nextResultPull{}},
				{owner: "willabides", repo: "semver-next", sha: sha3, err: assert.AnError},
			}),
		}
		_, err := getNext(ctx, &nextOptions{
			repo: "willabides/semver-next",
			base: "v0.15.0",
			head: sha1,
			gh:   &gh,
		})
		require.EqualError(t, err, errors.Join(assert.AnError, assert.AnError).Error())
	})

	t.Run("invalid minBump", func(t *testing.T) {
		_, err := getNext(ctx, &nextOptions{minBump: "foo"})
		require.EqualError(t, err, "invalid change level: foo")
	})

	t.Run("invalid maxBump", func(t *testing.T) {
		_, err := getNext(ctx, &nextOptions{maxBump: "foo"})
		require.EqualError(t, err, "invalid change level: foo")
	})

	t.Run("prev version not valid semver", func(t *testing.T) {
		_, err := getNext(ctx, &nextOptions{prevVersion: "foo"})
		require.EqualError(t, err, `invalid previous version "foo": Invalid Semantic Version`)
	})

	t.Run("invalid repo", func(t *testing.T) {
		_, err := getNext(ctx, &nextOptions{repo: "foo", prevVersion: "1.2.3"})
		require.EqualError(t, err, `repo must be in the form owner/name`)
	})

	t.Run("minBump > maxBump", func(t *testing.T) {
		_, err := getNext(ctx, &nextOptions{minBump: "major", maxBump: "minor"})
		require.EqualError(t, err, "minBump must be less than or equal to maxBump")
	})
}
