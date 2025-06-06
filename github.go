package main

import (
	"context"

	"github.com/willabides/release-train/v3/internal/github"
)

//go:generate go run go.uber.org/mock/mockgen@v0.5.2 -source=$GOFILE -destination=internal/mocks/$GOFILE -package mocks -write_package_comment=false

type GithubClient interface {
	ListMergedPullsForCommit(ctx context.Context, owner, repo, sha string) ([]github.BasePull, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string, count int) (*github.CommitComparison, error)
	GenerateReleaseNotes(ctx context.Context, owner, repo, tag, prevTag string) (string, error)
	CreateRelease(ctx context.Context, owner, repo, tag, body string, prerelease bool) (*github.RepoRelease, error)
	UploadAsset(ctx context.Context, uploadURL, filename string) error
	DeleteRelease(ctx context.Context, owner, repo string, id int64) error
	PublishRelease(ctx context.Context, owner, repo string, id int64) error
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.BasePull, error)
	GetPullRequestCommits(ctx context.Context, owner, repo string, number int) ([]string, error)
}
