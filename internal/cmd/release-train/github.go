package releasetrain

import (
	"context"
	"fmt"

	"github.com/willabides/release-train-action/v2/internal"
)

type githubClientConfig struct {
	GithubToken  string `kong:"hidden,required,env=GITHUB_TOKEN,help=${github_token_help}"`
	GithubApiUrl string `kong:"help=${github_api_url_help},default=${github_api_url_default}"`
}

func (c *githubClientConfig) Client(ctx context.Context) (internal.GithubClient, error) {
	return internal.NewGithubClient(ctx, c.GithubApiUrl, c.GithubToken, fmt.Sprintf("release-train/%s", getVersion(ctx)))
}
