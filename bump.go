package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type bumpContext struct {
	prePulls      []string
	nonPrePulls   []string
	stablePulls   []string
	unstablePulls []string
	isPre         bool
	isStable      bool
	prePrefix     string
}

func (c *bumpContext) addPull(pull ghPull) error {
	// Always add to stable/unstable tracking
	c.addStableCheck(pull)

	if !pull.HasPreLabel {
		// Handle non-pre pulls with change level
		if pull.ChangeLevel > changeLevelNone {
			c.nonPrePulls = append(c.nonPrePulls, fmt.Sprintf("#%d", pull.Number))
		}
		return nil
	}

	// Handle pre-release pulls
	c.isPre = true
	c.prePulls = append(c.prePulls, fmt.Sprintf("#%d", pull.Number))

	if pull.PreReleasePrefix == "" {
		return nil
	}

	if c.prePrefix == "" {
		c.prePrefix = pull.PreReleasePrefix
		return nil
	}

	if c.prePrefix != pull.PreReleasePrefix {
		return fmt.Errorf(
			"cannot have multiple pre-release prefixes in the same release. pre-release prefix. release contains both %q and %q",
			c.prePrefix, pull.PreReleasePrefix)
	}

	return nil
}

func (c *bumpContext) addStableCheck(pull ghPull) {
	if pull.HasStableLabel {
		c.isStable = true
		c.stablePulls = append(c.stablePulls, fmt.Sprintf("#%d", pull.Number))
		return
	}
	c.unstablePulls = append(c.unstablePulls, fmt.Sprintf("#%d", pull.Number))
}

func bumpVersion(
	ctx context.Context,
	prev semver.Version,
	minBump, maxBump changeLevel,
	commits []gitCommit,
) (*getNextResult, error) {
	logger := getLogger(ctx)
	logger.Debug("starting bumpVersion", slog.String("prev", prev.String()))
	if maxBump == 0 {
		maxBump = changeLevelMajor
	}
	result := getNextResult{
		PreviousVersion: prev,
	}
	pullsMap := map[int]ghPull{}
	for _, c := range commits {
		level := c.changeLevel()
		if level > result.ChangeLevel {
			result.ChangeLevel = level
		}
		for _, p := range c.Pulls {
			pullsMap[p.Number] = p
		}
	}
	logger.Debug("mapped pulls", slog.Any("result", result))
	if len(pullsMap) == 0 {
		result.NextVersion = result.PreviousVersion
		return &result, nil
	}
	pulls := make([]ghPull, 0, len(pullsMap))
	for _, p := range pullsMap {
		pulls = append(pulls, p)
	}
	sort.Slice(pulls, func(i, j int) bool {
		return pulls[i].Number < pulls[j].Number
	})
	var bumpCtx bumpContext
	for _, pull := range pulls {
		err := bumpCtx.addPull(pull)
		if err != nil {
			return nil, err
		}
	}
	if bumpCtx.isPre && len(bumpCtx.nonPrePulls) > 0 {
		return nil, fmt.Errorf("cannot have pre-release and non-pre-release PRs in the same release. pre-release PRs: %v, non-pre-release PRs: %v", bumpCtx.prePulls, bumpCtx.nonPrePulls)
	}
	if prev.Prerelease() != "" && bumpCtx.isStable && len(bumpCtx.unstablePulls) > 0 {
		return nil, fmt.Errorf("in order to release a stable version, all PRs must be labeled as stable. stable PRs: %v, unstable PRs: %v", bumpCtx.stablePulls, bumpCtx.unstablePulls)
	}
	if result.ChangeLevel < minBump && len(commits) > 0 {
		result.ChangeLevel = minBump
	}
	if result.ChangeLevel > maxBump {
		result.ChangeLevel = maxBump
	}
	if bumpCtx.isPre {
		next, err := incrPre(prev, result.ChangeLevel, bumpCtx.prePrefix)
		if err != nil {
			return nil, err
		}
		result.NextVersion = next
		return &result, nil
	}
	if prev.Prerelease() != "" && !bumpCtx.isStable {
		return nil, fmt.Errorf("cannot create a stable release from a pre-release unless all PRs are labeled semver:stable. unlabeled PRs: %v", bumpCtx.unstablePulls)
	}
	result.NextVersion = prev
	if bumpCtx.isStable {
		var err error
		result.NextVersion, err = result.NextVersion.SetPrerelease("")
		if err != nil {
			return nil, err
		}
		logger.Debug("isStable", slog.Any("result", result))
	}
	result.NextVersion = incrLevel(result.NextVersion, result.ChangeLevel)
	return &result, nil
}

func incrLevel(prev semver.Version, level changeLevel) semver.Version {
	switch level {
	case changeLevelNone:
		return prev
	case changeLevelPatch:
		return prev.IncPatch()
	case changeLevelMinor:
		return prev.IncMinor()
	case changeLevelMajor:
		return prev.IncMajor()
	default:
		panic(fmt.Sprintf("unknown change level %v", level))
	}
}

func incrPre(prev semver.Version, level changeLevel, prefix string) (next semver.Version, errOut error) {
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

	if level == changeLevelNone {
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
	case changeLevelMinor:
		needsIncr = prev.Patch() > 0
	case changeLevelMajor:
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
