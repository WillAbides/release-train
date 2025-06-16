package main

import (
	"context"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type getPrevTagOpts struct {
	Head        string
	RepoDir     string
	Prefixes    []string
	Fallback    string
	Constraints *semver.Constraints
}

func getPrevTag(ctx context.Context, options *getPrevTagOpts) (string, error) {
	if options == nil {
		options = &getPrevTagOpts{}
	}
	head := options.Head
	if head == "" {
		head = "HEAD"
	}
	prefixes := options.Prefixes
	if len(prefixes) == 0 {
		prefixes = []string{""}
	}
	cmdLine := []string{"git", "rev-list", "--pretty=%D", head}
	type prefixedVersion struct {
		prefix string
		ver    *semver.Version
	}
	var versions []prefixedVersion
	done := false
	err := runCmdHandleLines(ctx, options.RepoDir, cmdLine, func(line string, cancel context.CancelFunc) {
		if done {
			return
		}
		refs := strings.Split(line, ", ")
		for _, r := range refs {
			var ok bool
			r, ok = strings.CutPrefix(r, "tag: ")
			if !ok {
				continue
			}
			for _, prefix := range options.Prefixes {
				r, ok = strings.CutPrefix(r, prefix)
				if !ok {
					continue
				}
				ver, err := semver.StrictNewVersion(r)
				if err != nil {
					continue
				}
				if options.Constraints != nil && !options.Constraints.Check(ver) {
					continue
				}
				versions = append(versions, prefixedVersion{prefix, ver})
			}
		}
		if len(versions) > 0 {
			cancel()
			done = true
		}
	})
	if err != nil {
		return "", err
	}
	// order first by version then by index of prefix in prefixes
	sort.Slice(versions, func(i, j int) bool {
		a, b := versions[i], versions[j]
		if !a.ver.Equal(b.ver) {
			return a.ver.GreaterThan(b.ver)
		}
		for _, prefix := range prefixes {
			if a.prefix == prefix {
				return b.prefix != prefix
			}
			if b.prefix == prefix {
				return false
			}
		}
		return false
	})
	if len(versions) == 0 {
		return options.Fallback, nil
	}
	winner := versions[0]
	return winner.prefix + winner.ver.Original(), nil
}

// getPrevStableTag finds the previous stable tag (no prerelease) in git history
func getPrevStableTag(ctx context.Context, options *getPrevTagOpts) (string, error) {
	if options == nil {
		options = &getPrevTagOpts{}
	}
	head := options.Head
	if head == "" {
		head = "HEAD"
	}
	prefixes := options.Prefixes
	if len(prefixes) == 0 {
		prefixes = []string{""}
	}
	cmdLine := []string{"git", "rev-list", "--pretty=%D", head}
	type prefixedVersion struct {
		prefix string
		ver    *semver.Version
	}
	var versions []prefixedVersion
	done := false
	err := runCmdHandleLines(ctx, options.RepoDir, cmdLine, func(line string, cancel context.CancelFunc) {
		if done {
			return
		}
		refs := strings.Split(line, ", ")
		for _, r := range refs {
			var ok bool
			r, ok = strings.CutPrefix(r, "tag: ")
			if !ok {
				continue
			}
			for _, prefix := range options.Prefixes {
				r, ok = strings.CutPrefix(r, prefix)
				if !ok {
					continue
				}
				ver, err := semver.StrictNewVersion(r)
				if err != nil {
					continue
				}
				// Only include stable versions (no prerelease)
				if ver.Prerelease() != "" {
					continue
				}
				if options.Constraints != nil && !options.Constraints.Check(ver) {
					continue
				}
				versions = append(versions, prefixedVersion{prefix, ver})
			}
		}
		if len(versions) > 0 {
			cancel()
			done = true
		}
	})
	if err != nil {
		return "", err
	}
	// order first by version then by index of prefix in prefixes
	sort.Slice(versions, func(i, j int) bool {
		a, b := versions[i], versions[j]
		if !a.ver.Equal(b.ver) {
			return a.ver.GreaterThan(b.ver)
		}
		for _, prefix := range prefixes {
			if a.prefix == prefix {
				return b.prefix != prefix
			}
			if b.prefix == prefix {
				return false
			}
		}
		return false
	})
	if len(versions) == 0 {
		return "", nil // Return empty string if no stable version found
	}
	winner := versions[0]
	return winner.prefix + winner.ver.Original(), nil
}
