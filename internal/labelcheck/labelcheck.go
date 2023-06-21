package labelcheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/willabides/release-train-action/v3/internal"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

type Options struct {
	GhClient     internal.GithubClient
	PrNumber     int
	RepoOwner    string
	RepoName     string
	LabelAliases map[string]string
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
		resolved := internal.ResolveLabel(label, opts.LabelAliases)
		_, ok := internal.LabelLevels[resolved]
		if ok {
			return nil
		}
	}
	wantLabels := append(maps.Keys(internal.LabelLevels), maps.Keys(opts.LabelAliases)...)
	slices.Sort(wantLabels)
	return fmt.Errorf("pull request is missing a label. wanted one of: %s", strings.Join(wantLabels, ", "))
}
