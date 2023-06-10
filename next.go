package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
)

type changeLevel int

const (
	changeLevelNoChange changeLevel = iota
	changeLevelPatch
	changeLevelMinor
	changeLevelMajor
)

func (l changeLevel) String() string {
	switch l {
	case changeLevelNoChange:
		return "no change"
	case changeLevelPatch:
		return "patch"
	case changeLevelMinor:
		return "minor"
	case changeLevelMajor:
		return "major"
	default:
		panic("invalid change level")
	}
}

func (l changeLevel) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", l.String())), nil
}

func parseChangeLevel(v string) (changeLevel, error) {
	switch strings.ToLower(v) {
	case "patch":
		return changeLevelPatch, nil
	case "minor":
		return changeLevelMinor, nil
	case "major":
		return changeLevelMajor, nil
	case "none", "no change":
		return changeLevelNoChange, nil
	default:
		return changeLevelNoChange, fmt.Errorf("invalid change level: %s", v)
	}
}

var labelLevels = map[string]changeLevel{
	"breaking":        changeLevelMajor,
	"breaking change": changeLevelMajor,
	"major":           changeLevelMajor,
	"semver:major":    changeLevelMajor,

	"enhancement":  changeLevelMinor,
	"minor":        changeLevelMinor,
	"semver:minor": changeLevelMinor,

	"bug":          changeLevelPatch,
	"fix":          changeLevelPatch,
	"patch":        changeLevelPatch,
	"semver:patch": changeLevelPatch,

	"no change":        changeLevelNoChange,
	"semver:none":      changeLevelNoChange,
	"semver:no change": changeLevelNoChange,
	"semver:nochange":  changeLevelNoChange,
	"semver:skip":      changeLevelNoChange,
}

type nextResult struct {
	NextVersion     string             `json:"next_version"`
	PreviousVersion string             `json:"previous_version"`
	ChangeLevel     changeLevel        `json:"change_level"`
	Commits         []nextResultCommit `json:"commits,omitempty"`
}

type nextResultCommit struct {
	Sha         string           `json:"sha"`
	ChangeLevel changeLevel      `json:"change_level"`
	Pulls       []nextResultPull `json:"pulls,omitempty"`
}

type nextResultPull struct {
	Number      int         `json:"number"`
	Labels      []string    `json:"labels,omitempty"`
	ChangeLevel changeLevel `json:"change_level"`
}

func getCommitPRs(ctx context.Context, gh wrapper, owner, repo, commitSha string) ([]nextResultPull, error) {
	result, err := gh.ListPullRequestsWithCommit(ctx, owner, repo, commitSha)
	if err != nil {
		return nil, err
	}
	for i := range result {
		filteredLabels := make([]string, 0, len(result[i].Labels))
		for _, l := range result[i].Labels {
			l = strings.ToLower(l)
			level, ok := labelLevels[l]
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

func compareCommits(ctx context.Context, gh wrapper, owner, repo, baseRef, headRef string) ([]nextResultCommit, error) {
	var result []nextResultCommit
	commitShas, err := gh.CompareCommits(ctx, owner, repo, baseRef, headRef)
	if err != nil {
		return nil, err
	}
	result = make([]nextResultCommit, len(commitShas))
	var wg sync.WaitGroup
	var errLock sync.Mutex
	for i := range commitShas {
		commitSha := commitShas[i]
		result[i] = nextResultCommit{Sha: commitSha}
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
	var commitsMissingLabels []nextResultCommit
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

type nextOptions struct {
	gh          wrapper
	repo        string
	prevVersion string
	base        string
	head        string
	minBump     string
	maxBump     string
}

func getNext(ctx context.Context, opts nextOptions) (*nextResult, error) {
	minBump := opts.minBump
	if minBump == "" {
		minBump = "no change"
	}
	maxBump := opts.maxBump
	if maxBump == "" {
		maxBump = "major"
	}
	minBumpLevel, err := parseChangeLevel(minBump)
	if err != nil {
		return nil, err
	}
	maxBumpLevel, err := parseChangeLevel(maxBump)
	if err != nil {
		return nil, err
	}
	prevVersion := opts.prevVersion
	if prevVersion == "" {
		prevVersion = opts.base
	}
	if minBumpLevel > maxBumpLevel {
		return nil, fmt.Errorf("minBump must be less than or equal to maxBump")
	}
	prev, err := semver.NewVersion(prevVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid previous version %q: %v", prevVersion, err)
	}
	repoParts := strings.Split(opts.repo, "/")
	if len(repoParts) != 2 {
		return nil, fmt.Errorf("repo must be in the form owner/name")
	}
	owner, repo := repoParts[0], repoParts[1]
	resultCommits, err := compareCommits(ctx, opts.gh, owner, repo, opts.base, opts.head)
	if err != nil {
		return nil, err
	}
	result := nextResult{
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
	case changeLevelNoChange:
		result.NextVersion = prev.String()
	case changeLevelPatch:
		result.NextVersion = prev.IncPatch().String()
	case changeLevelMinor:
		result.NextVersion = prev.IncMinor().String()
	case changeLevelMajor:
		result.NextVersion = prev.IncMajor().String()
	}
	return &result, nil
}
