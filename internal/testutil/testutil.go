package testutil

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/willabides/release-train-action/v3/internal"
)

type GithubStub struct {
	StubListPullRequestsWithCommit func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error)
	StubCompareCommits             func(ctx context.Context, owner, repo, base, head string) ([]string, error)
	StubGenerateReleaseNotes       func(ctx context.Context, owner, repo, tag, prevTag string) (string, error)
	StubCreateRelease              func(ctx context.Context, owner, repo, tag, body string, prerelease bool) (*internal.RepoRelease, error)
	StubUploadAsset                func(ctx context.Context, uploadURL, filename string) error
	StubDeleteRelease              func(ctx context.Context, owner, repo string, id int64) error
	StubPublishRelease             func(ctx context.Context, owner, repo string, id int64) error
	StubGetPullRequest             func(ctx context.Context, owner, repo string, number int) (*internal.BasePull, error)
}

var _ internal.GithubClient = &GithubStub{}

func (w *GithubStub) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
	return w.StubListPullRequestsWithCommit(ctx, owner, repo, sha)
}

func (w *GithubStub) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error) {
	return w.StubCompareCommits(ctx, owner, repo, base, head)
}

func (w *GithubStub) GenerateReleaseNotes(ctx context.Context, owner, repo, tag, prevTag string) (string, error) {
	return w.StubGenerateReleaseNotes(ctx, owner, repo, tag, prevTag)
}

func (w *GithubStub) CreateRelease(ctx context.Context, owner, repo, tag, body string, prerelease bool) (*internal.RepoRelease, error) {
	return w.StubCreateRelease(ctx, owner, repo, tag, body, prerelease)
}

func (w *GithubStub) UploadAsset(ctx context.Context, uploadURL, filename string) error {
	return w.StubUploadAsset(ctx, uploadURL, filename)
}

func (w *GithubStub) DeleteRelease(ctx context.Context, owner, repo string, id int64) error {
	return w.StubDeleteRelease(ctx, owner, repo, id)
}

func (w *GithubStub) PublishRelease(ctx context.Context, owner, repo string, id int64) error {
	return w.StubPublishRelease(ctx, owner, repo, id)
}

func (w *GithubStub) GetPullRequest(ctx context.Context, owner, repo string, number int) (*internal.BasePull, error) {
	return w.StubGetPullRequest(ctx, owner, repo, number)
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
