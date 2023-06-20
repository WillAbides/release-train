package releasetrain

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/willabides/release-train-action/v3/internal"
)

type rootCmd struct {
	CheckoutDir string            `kong:"short=C,default='.',help=${checkout_dir_help}"`
	Label       map[string]string `kong:"help=${label_help}"`
	Release     releaseCmd        `kong:"cmd,help='Release a new version.'"`
	Action      actionCmd         `kong:"cmd"`
	Version     kong.VersionFlag

	GithubToken  string `kong:"hidden,required,env=GITHUB_TOKEN,help=${github_token_help}"`
	GithubApiUrl string `kong:"help=${github_api_url_help},default=${github_api_url_default}"`
}

func (c *rootCmd) GithubClient(ctx context.Context) (internal.GithubClient, error) {
	return internal.NewGithubClient(ctx, c.GithubApiUrl, c.GithubToken, fmt.Sprintf("release-train/%s", getVersion(ctx)))
}
