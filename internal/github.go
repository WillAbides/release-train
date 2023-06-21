package internal

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
)

type BasePull struct {
	Number int
	Labels []string
}

type RepoRelease struct {
	ID        int64
	UploadURL string
}

type GithubClient interface {
	ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]BasePull, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error)
	GenerateReleaseNotes(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error)
	CreateRelease(ctx context.Context, owner, repo string, release *github.RepositoryRelease) (*RepoRelease, error)
	UploadAsset(ctx context.Context, uploadURL, filename string, opts *github.UploadOptions) error
	DeleteRelease(ctx context.Context, owner, repo string, id int64) error
	PublishRelease(ctx context.Context, owner, repo string, id int64) error
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error)
}

func NewGithubClient(ctx context.Context, baseUrl, token, userAgent string) (GithubClient, error) {
	oauthClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	))
	rateLimitClient, err := github_ratelimit.NewRateLimitWaiterClient(oauthClient.Transport)
	if err != nil {
		return nil, err
	}
	// no need for uploadURL because if we upload release artifacts we will use release.UploadURL
	client, err := github.NewEnterpriseClient(baseUrl, "", rateLimitClient)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		client.UserAgent = userAgent
	}
	return &ghClient{Client: client}, nil
}

type ghClient struct {
	Client *github.Client
}

var _ GithubClient = &ghClient{}

// UploadAsset is largely copied from github.Client.UploadReleaseAsset. It is modified to use uploadURL instead of
// building it from releaseID so that we don't need to set upload url. It also accepts a filename instead of an
// *os.File.
func (g *ghClient) UploadAsset(ctx context.Context, uploadURL, filename string, opts *github.UploadOptions) error {
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
	g.Client.UploadURL = baseUrl

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

	if opts == nil {
		opts = &github.UploadOptions{}
	}
	if opts.Name == "" {
		opts.Name = filepath.Base(file.Name())
	}

	_, _, err = g.Client.Repositories.UploadReleaseAsset(ctx, owner, repo, idInt, opts, file)
	return err
}

func (g *ghClient) ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]BasePull, error) {
	var result []BasePull
	opts := &github.PullRequestListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		apiPulls, resp, err := g.Client.PullRequests.ListPullRequestsWithCommit(ctx, owner, repo, sha, opts)
		if err != nil {
			return nil, err
		}
		for _, apiPull := range apiPulls {
			if apiPull.GetMergedAt().IsZero() {
				continue
			}
			resultPull := BasePull{
				Number: apiPull.GetNumber(),
				Labels: make([]string, len(apiPull.Labels)),
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

func (g *ghClient) CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error) {
	var result []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		comp, resp, err := g.Client.Repositories.CompareCommits(ctx, owner, repo, base, head, opts)
		if err != nil {
			return nil, err
		}
		for _, commit := range comp.Commits {
			result = append(result, commit.GetSHA())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (g *ghClient) GenerateReleaseNotes(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
	comp, _, err := g.Client.Repositories.GenerateReleaseNotes(ctx, owner, repo, opts)
	if err != nil {
		return "", err
	}
	return comp.Body, nil
}

func (g *ghClient) CreateRelease(ctx context.Context, owner, repo string, release *github.RepositoryRelease) (*RepoRelease, error) {
	rel, _, err := g.Client.Repositories.CreateRelease(ctx, owner, repo, release)
	if err != nil {
		return nil, err
	}
	return &RepoRelease{
		ID:        rel.GetID(),
		UploadURL: rel.GetUploadURL(),
	}, nil
}

func (g *ghClient) DeleteRelease(ctx context.Context, owner, repo string, id int64) error {
	_, err := g.Client.Repositories.DeleteRelease(ctx, owner, repo, id)
	return err
}

func (g *ghClient) PublishRelease(ctx context.Context, owner, repo string, id int64) error {
	_, _, err := g.Client.Repositories.EditRelease(ctx, owner, repo, id, &github.RepositoryRelease{
		Draft: github.Bool(false),
	})
	return err
}

func (g *ghClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	pull, _, err := g.Client.PullRequests.Get(ctx, owner, repo, number)
	return pull, err
}
