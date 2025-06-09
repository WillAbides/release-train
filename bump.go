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

//nolint:gocyclo // TODO: refactor this
func bumpVersion(
	ctx context.Context,
	prev semver.Version,
	minBump, maxBump changeLevel,
	commits []gitCommit,
	forcePrerelease, forceStable bool,
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
		if forceStable && result.PreviousVersion.Prerelease() != "" {
			var err error
			result.NextVersion, err = result.PreviousVersion.SetPrerelease("")
			if err != nil {
				return nil, fmt.Errorf("failed to strip prerelease from version %s: %w", result.PreviousVersion.String(), err)
			}
		}
		// ChangeLevel is already changeLevelNone by default if no commits
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
	if forcePrerelease && len(bumpCtx.stablePulls) > 0 {
		return nil, fmt.Errorf("cannot force pre-release with stable PRs. stable PRs: %v", bumpCtx.stablePulls)
	}

	// Adjust changeLevel by min/maxBump before pre-release checks
	if result.ChangeLevel < minBump && len(commits) > 0 {
		result.ChangeLevel = minBump
	}
	if result.ChangeLevel > maxBump {
		result.ChangeLevel = maxBump
	}

	// Determine if making a pre-release
	makingPrerelease := !forceStable && (bumpCtx.isPre || forcePrerelease)
	if makingPrerelease {
		next, err := incrPre(prev, result.ChangeLevel, bumpCtx.prePrefix)
		if err != nil {
			return nil, err
		}
		result.NextVersion = next
		return &result, nil
	}

	// At this point, we are NOT making a pre-release.
	// Either forceStable is true, or PRs do not indicate pre-release.
	// This means we are aiming for a stable version.

	// Error if transitioning from pre-release to stable with unstable PRs, ONLY IF NOT forceStable.
	if prev.Prerelease() != "" && !forceStable {
		if len(bumpCtx.unstablePulls) > 0 {
			if bumpCtx.isStable {
				// Some PRs are stable-labeled, but not all.
				return nil, fmt.Errorf("in order to release a stable version, all PRs must be labeled as stable. stable PRs: %v, unstable PRs: %v", bumpCtx.stablePulls, bumpCtx.unstablePulls)
			}
			// else: !bumpCtx.isStable. This means no PRs are stable-labeled.
			return nil, fmt.Errorf("cannot create a stable release from a pre-release unless all PRs are labeled semver:stable. unlabeled PRs: %v", bumpCtx.unstablePulls)
		}
	}

	result.NextVersion = prev
	shouldIncrementVersionNumber := true // Default to incrementing the version number part

	if prev.Prerelease() != "" { // If previous was a pre-release
		var err error
		result.NextVersion, err = result.NextVersion.SetPrerelease("") // Make base stable
		if err != nil {
			return nil, fmt.Errorf("failed to strip prerelease for stable version from %s: %w", prev.String(), err)
		}
		logger.Debug("made stable from pre-release", slog.String("baseVersion", result.NextVersion.String()))

		if forceStable {
			// If forcing stable from a pre-release:
			// - If there are unstable PRs, the version number itself does not increment, only becomes stable.
			// - ChangeLevel still reflects the PRs' impact.
			// - If all PRs are stable (or no relevant PRs), then increment.
			if len(bumpCtx.unstablePulls) > 0 {
				shouldIncrementVersionNumber = false
				logger.Debug("forceStable from pre-release with unstable PRs: version number will not be incremented",
					slog.Any("unstablePulls", bumpCtx.unstablePulls),
					slog.String("nextVersion", result.NextVersion.String()))
			}
		}
		// If !forceStable and prev was pre-release, the error check above ensures all PRs are stable (or would have errored),
		// so increment is appropriate in that path.
	} else if forceStable { // If previous was stable, and forceStable is true
		// Ensure it's stable (e.g. if PRs might have suggested pre-release but forceStable overrides)
		// This might be redundant if prev was already stable and had no pre-release string, but SetPrerelease("") is idempotent.
		var err error
		result.NextVersion, err = result.NextVersion.SetPrerelease("")
		if err != nil {
			return nil, fmt.Errorf("failed to ensure stable version for %s with forceStable: %w", prev.String(), err)
		}
		logger.Debug("ensured stable due to forceStable on a previously stable version", slog.String("baseVersion", result.NextVersion.String()))
	}

	if shouldIncrementVersionNumber {
		result.NextVersion = incrLevel(result.NextVersion, result.ChangeLevel)
	}

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
