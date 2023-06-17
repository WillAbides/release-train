package testutil

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/go-github/v53/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train-action/v3/internal"
)

type GithubStub struct {
	StubListPullRequestsWithCommit func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error)
	StubCompareCommits             func(ctx context.Context, owner, repo, base, head string) ([]string, error)
	StubGenerateReleaseNotes       func(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error)
	StubCreateRelease              func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) (*github.RepositoryRelease, error)
	StubUploadAsset                func(ctx context.Context, uploadURL, filename string, opts *github.UploadOptions) error
}

var _ internal.GithubClient = &GithubStub{}

func (w *GithubStub) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
	return w.StubListPullRequestsWithCommit(ctx, owner, repo, sha)
}

func (w *GithubStub) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error) {
	return w.StubCompareCommits(ctx, owner, repo, base, head)
}

func (w *GithubStub) GenerateReleaseNotes(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
	return w.StubGenerateReleaseNotes(ctx, owner, repo, opts)
}

func (w *GithubStub) CreateRelease(ctx context.Context, owner, repo string, release *github.RepositoryRelease) (*github.RepositoryRelease, error) {
	return w.StubCreateRelease(ctx, owner, repo, release)
}

func (w *GithubStub) UploadAsset(ctx context.Context, uploadURL, filename string, opts *github.UploadOptions) error {
	return w.StubUploadAsset(ctx, uploadURL, filename, opts)
}

type ListPullRequestsWithCommitCall struct {
	Owner, Repo, Sha string
	Result           []internal.BasePull
	Err              error
}

func MockListPullRequestsWithCommit(t *testing.T, calls []ListPullRequestsWithCommitCall) func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
	var lock sync.Mutex
	return func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
		lock.Lock()
		defer lock.Unlock()
		idx := 0
		for ; idx < len(calls); idx++ {
			if calls[idx].Owner == owner && calls[idx].Repo == repo && calls[idx].Sha == sha {
				break
			}
		}
		if !assert.Less(t, idx, len(calls), "unexpected call to ListPullRequestsWithCommit") {
			return nil, fmt.Errorf("unexpected call to ListPullRequestsWithCommit")
		}
		call := calls[idx]
		calls = append(calls[:idx], calls[idx+1:]...)
		return call.Result, call.Err
	}
}

func MustNewPull(t *testing.T, number int, labels ...string) internal.Pull {
	t.Helper()
	p, err := internal.NewPull(number, labels...)
	require.NoError(t, err)
	return *p
}
