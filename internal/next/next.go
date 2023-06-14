package next

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/willabides/release-train-action/v2/internal"
)

var LabelLevels = map[string]internal.ChangeLevel{
	"breaking":        internal.ChangeLevelMajor,
	"breaking change": internal.ChangeLevelMajor,
	"major":           internal.ChangeLevelMajor,
	"semver:major":    internal.ChangeLevelMajor,

	"enhancement":  internal.ChangeLevelMinor,
	"minor":        internal.ChangeLevelMinor,
	"semver:minor": internal.ChangeLevelMinor,

	"bug":          internal.ChangeLevelPatch,
	"fix":          internal.ChangeLevelPatch,
	"patch":        internal.ChangeLevelPatch,
	"semver:patch": internal.ChangeLevelPatch,

	"no change":        internal.ChangeLevelNoChange,
	"semver:none":      internal.ChangeLevelNoChange,
	"semver:no change": internal.ChangeLevelNoChange,
	"semver:nochange":  internal.ChangeLevelNoChange,
	"semver:skip":      internal.ChangeLevelNoChange,
}

type Result struct {
	NextVersion     string               `json:"next_version"`
	PreviousVersion string               `json:"previous_version"`
	ChangeLevel     internal.ChangeLevel `json:"change_level"`
	Commits         []Commit             `json:"commits,omitempty"`
}

type Commit struct {
	Sha         string               `json:"sha"`
	ChangeLevel internal.ChangeLevel `json:"change_level"`
	Pulls       []internal.Pull      `json:"pulls,omitempty"`
}

func getCommitPRs(ctx context.Context, gh ghClient, owner, repo, commitSha string) ([]internal.Pull, error) {
	result, err := gh.ListPullRequestsWithCommit(ctx, owner, repo, commitSha)
	if err != nil {
		return nil, err
	}
	for i := range result {
		filteredLabels := make([]string, 0, len(result[i].Labels))
		for _, l := range result[i].Labels {
			l = strings.ToLower(l)
			level, ok := LabelLevels[l]
			if !ok {
				continue
			}
			filteredLabels = append(filteredLabels, l)
			if level > result[i].ChangeLevel {
				result[i].ChangeLevel = level
			}
		}
		result[i].Labels = filteredLabels
	}
	return result, nil
}

func compareCommits(ctx context.Context, gh ghClient, owner, repo, baseRef, headRef string) ([]Commit, error) {
	var result []Commit
	commitShas, err := gh.CompareCommits(ctx, owner, repo, baseRef, headRef)
	if err != nil {
		return nil, err
	}
	result = make([]Commit, len(commitShas))
	var wg sync.WaitGroup
	var errLock sync.Mutex
	for i := range commitShas {
		commitSha := commitShas[i]
		result[i] = Commit{Sha: commitSha}
		wg.Add(1)
		go func(idx int) {
			var e error
			result[idx].Pulls, e = getCommitPRs(ctx, gh, owner, repo, commitSha)
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
	var commitsMissingLabels []Commit
	for i := range result {
		hasLabel := false
		for _, p := range result[i].Pulls {
			if len(p.Labels) > 0 {
				hasLabel = true
			}
			if p.ChangeLevel > result[i].ChangeLevel {
				result[i].ChangeLevel = p.ChangeLevel
			}
		}
		if len(result[i].Pulls) > 0 && !hasLabel {
			commitsMissingLabels = append(commitsMissingLabels, result[i])
		}
	}
	if len(commitsMissingLabels) > 0 {
		var commitMsgs []string
		for _, c := range commitsMissingLabels {
			var prNumbers []string
			for _, p := range c.Pulls {
				prNumbers = append(prNumbers, fmt.Sprintf("#%d", p.Number))
			}
			commitMsgs = append(commitMsgs, fmt.Sprintf("%s (%s)", c.Sha, strings.Join(prNumbers, ", ")))
		}
		return nil, fmt.Errorf("commits with no semver labels on associated PRs:\n%s", strings.Join(commitMsgs, "\n"))
	}
	return result, nil
}

type ghClient interface {
	ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]internal.Pull, error)
	CompareCommits(ctx context.Context, owner, repo, base, head string) ([]string, error)
}

type Options struct {
	GithubClient ghClient
	Repo         string
	PrevVersion  string
	Base         string
	Head         string
	MinBump      string
	MaxBump      string
}

func GetNext(ctx context.Context, opts *Options) (*Result, error) {
	if opts == nil {
		opts = &Options{}
	}
	minBump := opts.MinBump
	if minBump == "" {
		minBump = "no change"
	}
	maxBump := opts.MaxBump
	if maxBump == "" {
		maxBump = "major"
	}
	minBumpLevel, err := internal.ParseChangeLevel(minBump)
	if err != nil {
		return nil, err
	}
	maxBumpLevel, err := internal.ParseChangeLevel(maxBump)
	if err != nil {
		return nil, err
	}
	prevVersion := opts.PrevVersion
	if prevVersion == "" {
		prevVersion = opts.Base
	}
	if minBumpLevel > maxBumpLevel {
		return nil, fmt.Errorf("minBump must be less than or equal to maxBump")
	}
	prev, err := semver.NewVersion(prevVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid previous version %q: %v", prevVersion, err)
	}
	repoParts := strings.Split(opts.Repo, "/")
	if len(repoParts) != 2 {
		return nil, fmt.Errorf("repo must be in the form owner/name")
	}
	owner, repo := repoParts[0], repoParts[1]
	resultCommits, err := compareCommits(ctx, opts.GithubClient, owner, repo, opts.Base, opts.Head)
	if err != nil {
		return nil, err
	}
	result := Result{
		Commits:         resultCommits,
		PreviousVersion: prev.String(),
	}
	for _, c := range resultCommits {
		if c.ChangeLevel > result.ChangeLevel {
			result.ChangeLevel = c.ChangeLevel
		}
	}
	if result.ChangeLevel < minBumpLevel && len(result.Commits) > 0 {
		result.ChangeLevel = minBumpLevel
	}
	if result.ChangeLevel > maxBumpLevel {
		result.ChangeLevel = maxBumpLevel
	}
	switch result.ChangeLevel {
	case internal.ChangeLevelNoChange:
		result.NextVersion = prev.String()
	case internal.ChangeLevelPatch:
		result.NextVersion = prev.IncPatch().String()
	case internal.ChangeLevelMinor:
		result.NextVersion = prev.IncMinor().String()
	case internal.ChangeLevelMajor:
		result.NextVersion = prev.IncMajor().String()
	}
	return &result, nil
}
