package main

import (
	"context"

	github2 "github.com/willabides/release-train/v3/internal/github"
)

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=$GOFILE -destination=mock_$GOFILE -package main -write_package_comment=false

type GithubClient interface {
	ListMergedPullsForCommit(ctx context.Context, owner, repo, sha string) ([]github2.BasePull, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string, count int) (*github2.CommitComparison, error)
	GenerateReleaseNotes(ctx context.Context, owner, repo, tag, prevTag string) (string, error)
	CreateRelease(ctx context.Context, owner, repo, tag, body string, prerelease bool) (*github2.RepoRelease, error)
	UploadAsset(ctx context.Context, uploadURL, filename string) error
	DeleteRelease(ctx context.Context, owner, repo string, id int64) error
	PublishRelease(ctx context.Context, owner, repo string, id int64) error
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*github2.BasePull, error)
	GetPullRequestCommits(ctx context.Context, owner, repo string, number int) ([]string, error)
}
