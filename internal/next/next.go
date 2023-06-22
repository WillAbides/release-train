package next

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/willabides/release-train-action/v3/internal"
)

type Result struct {
	NextVersion     semver.Version       `json:"next_version"`
	PreviousVersion semver.Version       `json:"previous_version"`
	ChangeLevel     internal.ChangeLevel `json:"change_level"`
}

func getCommitPRs(ctx context.Context, opts *Options, commitSha string) ([]internal.Pull, error) {
	ghResult, err := opts.GithubClient.ListPullRequestsWithCommit(ctx, opts.owner(), opts.repo(), commitSha)
	if err != nil {
		return nil, err
	}
	result := make([]internal.Pull, 0, len(ghResult))
	for _, r := range ghResult {
		p, e := internal.NewPull(r.Number, opts.LabelAliases, r.Labels...)
		if e != nil {
			return nil, e
		}
		result = append(result, *p)
	}
	return result, nil
}

func compareCommits(ctx context.Context, opts *Options) ([]Commit, error) {
	var result []Commit
	commitShas, err := opts.GithubClient.CompareCommits(ctx, opts.owner(), opts.repo(), opts.Base, opts.Head)
	if err != nil {
		return nil, err
	}
	result = make([]Commit, len(commitShas))
	var wg sync.WaitGroup
	var errLock sync.Mutex
	for i := range commitShas {
		commitSha := commitShas[i]
		result[i].Sha = commitSha
		wg.Add(1)
		go func(idx int) {
			var e error
			result[idx].Pulls, e = getCommitPRs(ctx, opts, commitSha)
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
	for i := range result {
		err = result[i].validate()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

type Options struct {
	GithubClient internal.GithubClient
	Repo         string
	PrevVersion  string
	Base         string
	Head         string
	MinBump      *internal.ChangeLevel
	MaxBump      *internal.ChangeLevel
	CheckPR      int
	LabelAliases map[string]string
}

func (o *Options) repo() string {
	_, repo, _ := strings.Cut(o.Repo, "/")
	return repo
}

func (o *Options) owner() string {
	owner, _, _ := strings.Cut(o.Repo, "/")
	return owner
}

func GetNext(ctx context.Context, opts *Options) (*Result, error) {
	if opts == nil {
		opts = &Options{}
	}
	minBump := internal.ChangeLevelNone
	if opts.MinBump != nil {
		minBump = *opts.MinBump
	}
	maxBump := internal.ChangeLevelMajor
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
	if opts.CheckPR != 0 {
		commits, err = includePullInResults(ctx, opts, commits)
		if err != nil {
			return nil, err
		}
	}
	return bumpVersion(*prev, minBump, maxBump, commits)
}

func includePullInResults(ctx context.Context, opts *Options, commits []Commit) ([]Commit, error) {
	base, err := opts.GithubClient.GetPullRequest(ctx, opts.owner(), opts.repo(), opts.CheckPR)
	if err != nil {
		return nil, err
	}
	pull, err := internal.NewPull(opts.CheckPR, opts.LabelAliases, base.Labels...)
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
	result := make([]Commit, 0, len(commits))
	for _, c := range commits {
		if lookup[c.Sha] {
			c.Pulls = append(c.Pulls, *pull)
		}
		result = append(result, c)
	}
	return result, nil
}

func bumpVersion(prev semver.Version, minBump, maxBump internal.ChangeLevel, commits []Commit) (*Result, error) {
	if maxBump == 0 {
		maxBump = internal.ChangeLevelMajor
	}
	result := Result{
		PreviousVersion: prev,
	}
	pullsMap := map[int]internal.Pull{}
	for _, c := range commits {
		level := c.changeLevel()
		if level > result.ChangeLevel {
			result.ChangeLevel = level
		}
		for _, p := range c.Pulls {
			pullsMap[p.Number] = p
		}
	}
	if len(pullsMap) == 0 {
		result.NextVersion = result.PreviousVersion
		return &result, nil
	}
	pulls := make([]internal.Pull, 0, len(pullsMap))
	for _, p := range pullsMap {
		pulls = append(pulls, p)
	}
	sort.Slice(pulls, func(i, j int) bool {
		return pulls[i].Number < pulls[j].Number
	})
	var prePulls, nonPrePulls, stablePulls, unstablePulls []string
	var isPre, isStable bool
	prePrefix := ""
	for _, pull := range pulls {
		if pull.HasPreLabel {
			isPre = true
			prePulls = append(prePulls, fmt.Sprintf("#%d", pull.Number))
			if pull.PreReleasePrefix != "" {
				if prePrefix == "" {
					prePrefix = pull.PreReleasePrefix
				}
				if prePrefix != pull.PreReleasePrefix {
					return nil, fmt.Errorf("cannot have multiple pre-release prefixes in the same release. pre-release prefix. release contains both %q and %q", prePrefix, pull.PreReleasePrefix)
				}
			}
		} else if pull.ChangeLevel > internal.ChangeLevelNone {
			nonPrePulls = append(nonPrePulls, fmt.Sprintf("#%d", pull.Number))
		}
		if pull.HasStableLabel {
			isStable = true
			stablePulls = append(stablePulls, fmt.Sprintf("#%d", pull.Number))
		} else {
			unstablePulls = append(unstablePulls, fmt.Sprintf("#%d", pull.Number))
		}
	}
	if isPre && len(nonPrePulls) > 0 {
		return nil, fmt.Errorf("cannot have pre-release and non-pre-release PRs in the same release. pre-release PRs: %v, non-pre-release PRs: %v", prePulls, nonPrePulls)
	}
	if prev.Prerelease() != "" && isStable && len(unstablePulls) > 0 {
		return nil, fmt.Errorf("in order to release a stable version, all PRs must be labeled as stable. stable PRs: %v, unstable PRs: %v", stablePulls, unstablePulls)
	}
	if result.ChangeLevel < minBump && len(commits) > 0 {
		result.ChangeLevel = minBump
	}
	if result.ChangeLevel > maxBump {
		result.ChangeLevel = maxBump
	}
	if isPre {
		next, err := incrPre(prev, result.ChangeLevel, prePrefix)
		if err != nil {
			return nil, err
		}
		result.NextVersion = next
		return &result, nil
	}
	if prev.Prerelease() != "" && !isStable {
		return nil, fmt.Errorf("cannot create a stable release from a pre-release unless all PRs are labeled semver:stable. unlabeled PRs: %v", unstablePulls)
	}
	result.NextVersion = incrLevel(prev, result.ChangeLevel)
	return &result, nil
}

func incrLevel(prev semver.Version, level internal.ChangeLevel) semver.Version {
	switch level {
	case internal.ChangeLevelNone:
		return prev
	case internal.ChangeLevelPatch:
		return prev.IncPatch()
	case internal.ChangeLevelMinor:
		return prev.IncMinor()
	case internal.ChangeLevelMajor:
		return prev.IncMajor()
	default:
		panic(fmt.Sprintf("unknown change level %v", level))
	}
}

func incrPre(prev semver.Version, level internal.ChangeLevel, prefix string) (next semver.Version, errOut error) {
	orig := prev

	// make sure result is always greater than prev
	defer func() {
		if errOut != nil {
			return
		}
		if !next.GreaterThan(&orig) {
			errOut = fmt.Errorf("pre-release version %q is not greater than %q", next, orig)
		}
	}()

	if level == internal.ChangeLevelNone {
		return prev, fmt.Errorf("invalid change level for pre-release: %v", level)
	}
	prevPre := prev.Prerelease()
	if prevPre == "" {
		pre := prefix + ".0"
		if pre == ".0" {
			pre = "0"
		}
		prev = incrLevel(prev, level)
		return prev.SetPrerelease(pre)
	}
	// make sure everything to the right of level is 0
	needsIncr := false
	switch level {
	case internal.ChangeLevelMinor:
		needsIncr = prev.Patch() > 0
	case internal.ChangeLevelMajor:
		needsIncr = prev.Minor() > 0 || prev.Patch() > 0
	}
	if needsIncr {
		prev = incrLevel(prev, level)
	}
	preParts := strings.Split(prevPre, ".")
	end, err := strconv.Atoi(preParts[len(preParts)-1])
	if err == nil {
		if needsIncr {
			end = -1
		}
		prevPre = strings.Join(preParts[:len(preParts)-1], ".")

		// when no prefix is specified or prefix matches prevPre, use the same prefix as the previous version
		if prefix == "" && prevPre == "" {
			return prev.SetPrerelease(strconv.Itoa(end + 1))
		}
		if prefix == prevPre || prefix == "" {
			return prev.SetPrerelease(prevPre + "." + strconv.Itoa(end+1))
		}

		// otherwise, use the specified prefix starting at 0
		return prev.SetPrerelease(prefix + "." + "0")
	}

	// if prefix isn't specified, use the same prefix as the previous version
	if prefix == "" {
		prefix = prevPre
	}

	return prev.SetPrerelease(prefix + ".0")
}
