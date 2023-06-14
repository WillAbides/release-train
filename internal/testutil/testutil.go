package testutil

import (
	"context"

	"github.com/google/go-github/v53/github"
	"github.com/willabides/release-train-action/v2/internal"
)

type GithubStub struct {
	StubListPullRequestsWithCommit func(ctx context.Context, owner, repo, sha string) ([]internal.Pull, error)
	StubCompareCommits             func(ctx context.Context, owner, repo, base, head string) ([]string, error)
	StubGenerateReleaseNotes       func(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error)
	StubCreateRelease              func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) error
}

var _ internal.GithubClient = &GithubStub{}

func (w *GithubStub) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]internal.Pull, error) {
	return w.StubListPullRequestsWithCommit(ctx, owner, repo, sha)
}

func (w *GithubStub) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error) {
	return w.StubCompareCommits(ctx, owner, repo, base, head)
}

func (w *GithubStub) GenerateReleaseNotes(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
	return w.StubGenerateReleaseNotes(ctx, owner, repo, opts)
}

func (w *GithubStub) CreateRelease(ctx context.Context, owner, repo string, release *github.RepositoryRelease) error {
	return w.StubCreateRelease(ctx, owner, repo, release)
}
