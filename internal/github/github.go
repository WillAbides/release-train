package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"

	ratelimit "github.com/gofri/go-github-ratelimit/v2/github_ratelimit"
	"github.com/google/go-github/v72/github"
)

type BasePull struct {
	Number         int
	MergeCommitSha string
	Labels         []string
}

type RepoRelease struct {
	ID        int64
	UploadURL string
}

type CommitComparison struct {
	AheadBy  int
	BehindBy int
	Commits  []string
}

func NewClient(baseUrl, token, userAgent string) (*Client, error) {
	transport := ratelimit.NewSecondaryLimiter(http.DefaultTransport)
	httpClient := &http.Client{Transport: transport}
	githubClient := github.NewClient(httpClient).WithAuthToken(token)

	// no need for uploadURL because if we upload release artifacts we will use release.UploadURL
	githubClient, err := githubClient.WithEnterpriseURLs(baseUrl, "")
	if err != nil {
		return nil, err
	}
	githubClient.UserAgent = userAgent
	return &Client{client: githubClient}, nil
}

type Client struct {
	client *github.Client
}

// UploadAsset is largely copied from github.Client.UploadReleaseAsset. It is modified to use uploadURL instead of
// building it from releaseID so that we don't need to set upload url. It also accepts a filename instead of an
// *os.File.
func (g *Client) UploadAsset(ctx context.Context, uploadURL, filename string) error {
	re := regexp.MustCompile(`^(?P<base>.+/)repos/(?P<owner>[^/]+)/(?P<repo>[^/]+)/releases/(?P<id>\d+)/assets`)
	matches := re.FindStringSubmatch(uploadURL)
	if len(matches) != 5 {
		return fmt.Errorf("invalid upload url: %s", uploadURL)
	}
	base := matches[1]
	owner := matches[2]
	repo := matches[3]
	id := matches[4]

	baseUrl, err := url.Parse(base)
	if err != nil {
		return err
	}
	g.client.UploadURL = baseUrl

	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	defer func() {
		//nolint:errcheck // ignore close error for read-only file
		_ = file.Close()
	}()

	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return err
	}

	opts := github.UploadOptions{
		Name: filepath.Base(file.Name()),
	}

	_, _, err = g.client.Repositories.UploadReleaseAsset(ctx, owner, repo, idInt, &opts, file)
	return err
}

func (g *Client) ListMergedPullsForCommit(ctx context.Context, owner, repo, sha string) ([]BasePull, error) {
	var result []BasePull
	opts := &github.ListOptions{PerPage: 100}
	for {
		apiPulls, resp, err := g.client.PullRequests.ListPullRequestsWithCommit(ctx, owner, repo, sha, opts)
		if err != nil {
			return nil, err
		}
		for _, apiPull := range apiPulls {
			mergeCommitSHA := apiPull.GetMergeCommitSHA()
			// only include merged PRs
			if mergeCommitSHA == "" {
				continue
			}
			resultPull := BasePull{
				Number:         apiPull.GetNumber(),
				Labels:         make([]string, len(apiPull.Labels)),
				MergeCommitSha: mergeCommitSHA,
			}
			for i, label := range apiPull.Labels {
				resultPull.Labels[i] = label.GetName()
			}
			result = append(result, resultPull)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

// CompareCommits returns a commit comparison that includes up to count commits. If count is -1, all commits are
// included. If count is 0, no commits are included.
func (g *Client) CompareCommits(
	ctx context.Context,
	owner, repo, base, head string,
	count int,
) (*CommitComparison, error) {
	var result CommitComparison
	const maxPerPage = 100
	opts := &github.ListOptions{PerPage: maxPerPage}
	if count >= 0 && count < maxPerPage {
		opts.PerPage = count
	}
	if opts.PerPage == 0 {
		opts.PerPage = 1
	}
	for {
		comp, resp, err := g.client.Repositories.CompareCommits(ctx, owner, repo, base, head, opts)
		if err != nil {
			return nil, err
		}
		result.AheadBy = comp.GetAheadBy()
		result.BehindBy = comp.GetBehindBy()
		if count == 0 {
			break
		}
		for _, commit := range comp.Commits {
			result.Commits = append(result.Commits, commit.GetSHA())
			if len(result.Commits) == count {
				break
			}
		}
		if len(result.Commits) == count || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return &result, nil
}

func (g *Client) GenerateReleaseNotes(ctx context.Context, owner, repo, tag, prevTag string) (string, error) {
	comp, _, err := g.client.Repositories.GenerateReleaseNotes(ctx, owner, repo, &github.GenerateNotesOptions{
		TagName:         tag,
		PreviousTagName: &prevTag,
	})
	if err != nil {
		return "", err
	}
	return comp.Body, nil
}

func (g *Client) CreateRelease(
	ctx context.Context,
	owner, repo, tag, body string,
	prerelease bool,
) (*RepoRelease, error) {
	rel, _, err := g.client.Repositories.CreateRelease(ctx, owner, repo, &github.RepositoryRelease{
		TagName:    &tag,
		Name:       &tag,
		Body:       &body,
		MakeLatest: github.Ptr("legacy"),
		Prerelease: &prerelease,
		Draft:      github.Ptr(true),
	})
	if err != nil {
		return nil, err
	}
	return &RepoRelease{
		ID:        rel.GetID(),
		UploadURL: rel.GetUploadURL(),
	}, nil
}

func (g *Client) DeleteRelease(ctx context.Context, owner, repo string, id int64) error {
	_, err := g.client.Repositories.DeleteRelease(ctx, owner, repo, id)
	return err
}

func (g *Client) PublishRelease(ctx context.Context, owner, repo, makeLatest string, id int64) error {
	release := github.RepositoryRelease{Draft: github.Ptr(false)}
	if !slices.Contains([]string{"", "legacy", "true", "false"}, makeLatest) {
		return fmt.Errorf("invalid makeLatest value: %s", makeLatest)
	}
	if makeLatest != "" {
		release.MakeLatest = &makeLatest
	}
	_, _, err := g.client.Repositories.EditRelease(ctx, owner, repo, id, &release)
	return err
}

func (g *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (*BasePull, error) {
	p, _, err := g.client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	pull := BasePull{
		Number: p.GetNumber(),
		Labels: make([]string, len(p.Labels)),
	}
	for i, label := range p.Labels {
		pull.Labels[i] = label.GetName()
	}
	return &pull, nil
}

func (g *Client) GetPullRequestCommits(ctx context.Context, owner, repo string, number int) ([]string, error) {
	var commitShas []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		apiCommits, resp, err := g.client.PullRequests.ListCommits(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, err
		}
		for _, apiCommit := range apiCommits {
			commitShas = append(commitShas, apiCommit.GetSHA())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return commitShas, nil
}
