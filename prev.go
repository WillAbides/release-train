package main

import (
	"context"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type getPrevTagOpts struct {
	Head       string
	RepoDir    string
	TagPrefix  string
	StableOnly bool
}

func getPrevTag(ctx context.Context, options *getPrevTagOpts) (string, error) {
	if options == nil {
		options = &getPrevTagOpts{}
	}
	head := options.Head
	if head == "" {
		head = "HEAD"
	}
	versions, err := revlistVersions(ctx, options, head)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", nil
	}
	winner := slices.MaxFunc(versions, (*semver.Version).Compare)
	return options.TagPrefix + winner.Original(), nil
}

func revlistVersions(ctx context.Context, options *getPrevTagOpts, head string) ([]*semver.Version, error) {
	cmdLine := []string{"git", "rev-list", "--pretty=%D", head}
	var versions []*semver.Version
	done := false
	err := runCmdHandleLines(ctx, options.RepoDir, cmdLine, func(line string, cancel context.CancelFunc) {
		if done {
			return
		}
		parsed := parseGitTagLine(line, options.TagPrefix, options.StableOnly)
		if len(parsed) > 0 {
			versions = append(versions, parsed...)
		}
		if len(versions) > 0 {
			cancel()
			done = true
		}
	})
	if err != nil {
		return nil, err
	}
	return versions, nil
}

func parseGitTagLine(line, prefix string, stableOnly bool) []*semver.Version {
	var result []*semver.Version
	refs := strings.Split(line, ", ")
	for _, r := range refs {
		tag, hasTag := strings.CutPrefix(r, "tag: ")
		if !hasTag {
			continue
		}
		stripped, hasPrefix := strings.CutPrefix(tag, prefix)
		if !hasPrefix {
			continue
		}
		ver, err := semver.StrictNewVersion(stripped)
		if err != nil {
			continue
		}
		if stableOnly && ver.Prerelease() != "" {
			continue
		}
		result = append(result, ver)
	}
	return result
}
