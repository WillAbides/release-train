package labelcheck

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/willabides/release-train-action/v3/internal"
)

type Options struct {
	GhClient  internal.GithubClient
	PrNumber  int
	RepoOwner string
	RepoName  string
}

func Check(ctx context.Context, opts *Options) error {
	if opts == nil {
		opts = &Options{}
	}
	pull, err := opts.GhClient.GetPullRequest(ctx, opts.RepoOwner, opts.RepoName, opts.PrNumber)
	if err != nil {
		return err
	}

	for _, label := range pull.Labels {
		name := strings.ToLower(label.GetName())
		_, ok := internal.LabelLevels[name]
		if ok {
			return nil
		}
	}
	var wantLabels []string
	for k := range internal.LabelLevels {
		wantLabels = append(wantLabels, k)
	}
	sort.Strings(wantLabels)
	return fmt.Errorf("pull request is missing a label. wanted one of: %s", strings.Join(wantLabels, ", "))
}
