package internal

import (
	"context"
	"mime"
	"net/url"
	"os"
	"path/filepath"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
	"github.com/google/go-github/v53/github"
	"github.com/google/go-querystring/query"
	"golang.org/x/oauth2"
)

type BasePull struct {
	Number int
	Labels []string
}

type GithubClient interface {
	ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]BasePull, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error)
	GenerateReleaseNotes(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error)
	CreateRelease(ctx context.Context, owner, repo string, release *github.RepositoryRelease) error
	UploadAsset(ctx context.Context, uploadURL, filename string, opts *github.UploadOptions) error
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
	if opts == nil {
		opts = &github.UploadOptions{}
	}
	u, err := url.Parse(uploadURL)
	if err != nil {
		return err
	}
	qs, err := query.Values(opts)
	if err != nil {
		return err
	}
	u.RawQuery = qs.Encode()

	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	mediaType := mime.TypeByExtension(filepath.Ext(file.Name()))
	if opts.MediaType != "" {
		mediaType = opts.MediaType
	}

	req, err := g.Client.NewUploadRequest(u.String(), file, stat.Size(), mediaType)
	if err != nil {
		return err
	}

	resp, err := g.Client.Do(ctx, req, nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
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

func (g *ghClient) CreateRelease(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) error {
	_, _, err := g.Client.Repositories.CreateRelease(ctx, owner, repo, opts)
	return err
}
