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
	Commits         []Commit             `json:"commits,omitempty"`
}

func getCommitPRs(ctx context.Context, gh ghClient, aliases map[string]string, owner, repo, commitSha string) ([]internal.Pull, error) {
	ghResult, err := gh.ListPullRequestsWithCommit(ctx, owner, repo, commitSha)
	if err != nil {
		return nil, err
	}
	result := make([]internal.Pull, 0, len(ghResult))
	for _, r := range ghResult {
		p, e := internal.NewPull(r.Number, aliases, r.Labels...)
		if e != nil {
			return nil, e
		}
		result = append(result, *p)
	}
	return result, nil
}

func compareCommits(ctx context.Context, gh ghClient, aliases map[string]string, owner, repo, baseRef, headRef string) ([]Commit, error) {
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
		result[i].Sha = commitSha
		wg.Add(1)
		go func(idx int) {
			var e error
			result[idx].Pulls, e = getCommitPRs(ctx, gh, aliases, owner, repo, commitSha)
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

type ghClient interface {
	ListPullRequestsWithCommit(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error)
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
	LabelAliases map[string]string
}

func GetNext(ctx context.Context, opts *Options) (*Result, error) {
	if opts == nil {
		opts = &Options{}
	}
	minBump := opts.MinBump
	if minBump == "" {
		minBump = internal.ChangeLevelNone.String()
	}
	maxBump := opts.MaxBump
	if maxBump == "" {
		maxBump = internal.ChangeLevelMajor.String()
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
	resultCommits, err := compareCommits(ctx, opts.GithubClient, opts.LabelAliases, owner, repo, opts.Base, opts.Head)
	if err != nil {
		return nil, err
	}
	return bumpVersion(*prev, minBumpLevel, maxBumpLevel, resultCommits)
}

func bumpVersion(prev semver.Version, minBump, maxBump internal.ChangeLevel, commits []Commit) (*Result, error) {
	if maxBump == 0 {
		maxBump = internal.ChangeLevelMajor
	}
	result := Result{
		Commits:         commits,
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
	if result.ChangeLevel < minBump && len(result.Commits) > 0 {
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
