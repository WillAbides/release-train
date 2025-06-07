package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/willabides/release-train/v3/internal/github"
)

type getNextResult struct {
	NextVersion     semver.Version `json:"next_version"`
	PreviousVersion semver.Version `json:"previous_version"`
	ChangeLevel     changeLevel    `json:"change_level"`
}

func getCommitPRs(
	ctx context.Context,
	opts *getNextOptions,
	commitSha string,
	checkAncestor func(string) bool,
) ([]ghPull, error) {
	ghResult, err := opts.GithubClient.ListMergedPullsForCommit(ctx, opts.owner(), opts.repo(), commitSha)
	if err != nil {
		return nil, err
	}
	result := make([]ghPull, 0, len(ghResult))
	for _, r := range ghResult {
		if !checkAncestor(r.MergeCommitSha) {
			continue
		}
		p, e := newPull(r.Number, opts.LabelAliases, r.Labels...)
		if e != nil {
			return nil, e
		}
		result = append(result, *p)
	}
	return result, nil
}

func compareCommits(ctx context.Context, opts *getNextOptions) ([]gitCommit, error) {
	var result []gitCommit
	comp, err := opts.GithubClient.CompareCommits(ctx, opts.owner(), opts.repo(), opts.Base, opts.Head, -1)
	if err != nil {
		return nil, err
	}
	ancestorLookup := map[string]bool{}
	var ancestorMux sync.RWMutex
	var ancestorErr error
	checkAncestor := func(sha string) bool {
		ancestorMux.RLock()
		b, ok := ancestorLookup[sha]
		ancestorMux.RUnlock()
		if ok {
			return b
		}
		ancestorMux.Lock()
		defer ancestorMux.Unlock()
		if ancestorErr != nil {
			return false
		}
		b, ok = ancestorLookup[sha]
		if ok {
			return b
		}
		var ancestorComp *github.CommitComparison
		ancestorComp, ancestorErr = opts.GithubClient.CompareCommits(ctx, opts.owner(), opts.repo(), sha, opts.Head, 0)
		if ancestorErr != nil {
			return false
		}
		ancestorLookup[sha] = ancestorComp.BehindBy == 0
		return ancestorLookup[sha]
	}
	result = make([]gitCommit, len(comp.Commits))
	var wg sync.WaitGroup
	var errLock sync.Mutex
	for i := range comp.Commits {
		commitSha := comp.Commits[i]
		result[i].Sha = commitSha
		wg.Add(1)
		go func(idx int) {
			var e error
			result[idx].Pulls, e = getCommitPRs(ctx, opts, commitSha, checkAncestor)
			errLock.Lock()
			err = errors.Join(err, e)
			errLock.Unlock()
			wg.Done()
		}(i)
	}
	wg.Wait()
	if err != nil {
		return nil, err
	}
	if ancestorErr != nil {
		return nil, ancestorErr
	}
	for i := range result {
		err = result[i].validate()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

type getNextOptions struct {
	GithubClient    GithubClient
	Repo            string
	PrevVersion     string
	Base            string
	Head            string
	MinBump         *changeLevel
	MaxBump         *changeLevel
	CheckPR         int
	LabelAliases    map[string]string
	ForcePrerelease bool
}

func (o *getNextOptions) repo() string {
	_, repo, _ := strings.Cut(o.Repo, "/")
	return repo
}

func (o *getNextOptions) owner() string {
	owner, _, _ := strings.Cut(o.Repo, "/")
	return owner
}

func getNext(ctx context.Context, opts *getNextOptions) (*getNextResult, error) {
	logger := getLogger(ctx)
	if opts == nil {
		opts = &getNextOptions{}
	}
	logger.Debug(
		"starting GetNext",
		slog.String("repo", opts.Repo),
		slog.String("base", opts.Base),
		slog.String("head", opts.Head),
		slog.Int("check_pr", opts.CheckPR),
	)
	minBump := changeLevelNone
	if opts.MinBump != nil {
		minBump = *opts.MinBump
	}
	maxBump := changeLevelMajor
	if opts.MaxBump != nil {
		maxBump = *opts.MaxBump
	}
	prevVersion := opts.PrevVersion
	if prevVersion == "" {
		prevVersion = opts.Base
	}
	if minBump > maxBump {
		return nil, fmt.Errorf("minBump must be less than or equal to maxBump")
	}
	prev, err := semver.NewVersion(prevVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid previous version %q: %v", prevVersion, err)
	}
	if opts.repo() == "" {
		return nil, fmt.Errorf("repo must be in the form owner/name")
	}
	commits, err := compareCommits(ctx, opts)
	if err != nil {
		return nil, err
	}
	logger.Debug("found commits", slog.Any("commits", commits))
	if opts.CheckPR != 0 {
		commits, err = includePullInResults(ctx, opts, commits)
		if err != nil {
			return nil, err
		}
		logger.Debug("found commits after including PR", slog.Any("commits", commits))
	}
	return bumpVersion(ctx, *prev, minBump, maxBump, commits, opts.ForcePrerelease)
}

func includePullInResults(ctx context.Context, opts *getNextOptions, commits []gitCommit) ([]gitCommit, error) {
	base, err := opts.GithubClient.GetPullRequest(ctx, opts.owner(), opts.repo(), opts.CheckPR)
	if err != nil {
		return nil, err
	}
	pull, err := newPull(opts.CheckPR, opts.LabelAliases, base.Labels...)
	if err != nil {
		return nil, err
	}
	pullCommits, err := opts.GithubClient.GetPullRequestCommits(ctx, opts.owner(), opts.repo(), opts.CheckPR)
	if err != nil {
		return nil, err
	}
	lookup := make(map[string]bool, len(pullCommits))
	for _, c := range pullCommits {
		lookup[c] = true
	}
	result := make([]gitCommit, 0, len(commits))
	for _, c := range commits {
		if lookup[c.Sha] {
			c.Pulls = append(c.Pulls, *pull)
		}
		result = append(result, c)
	}
	return result, nil
}
