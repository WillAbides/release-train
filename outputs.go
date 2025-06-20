package main

import (
	"strconv"
)

type outputItem struct {
	name        string
	description string
	value       func(*Result) string
}

// outputItems defines the list of all possible action outputs.
func outputItems() []outputItem {
	return []outputItem{
		{
			name:        "previous-ref",
			description: `A git ref pointing to the previous release, or the current ref if no previous release can be found.`,
			value:       func(r *Result) string { return r.PreviousRef },
		},
		{
			name:        "previous-version",
			description: `The previous version on the release branch.`,
			value:       func(r *Result) string { return r.PreviousVersion },
		},
		{
			name:        "first-release",
			description: `Whether this is the first release on the release branch. Either "true" or "false".`,
			value:       func(r *Result) string { return strconv.FormatBool(r.FirstRelease) },
		},
		{
			name:        "release-version",
			description: `The version of the new release. Empty if no release is called for.`,
			value:       func(r *Result) string { return r.ReleaseVersion.String() },
		},
		{
			name:        "release-tag",
			description: `The tag of the new release. Empty if no release is called for.`,
			value:       func(r *Result) string { return r.ReleaseTag },
		},
		{
			name:        "change-level",
			description: `The level of change in the release. Either "major", "minor", "patch" or "none".`,
			value:       func(r *Result) string { return r.ChangeLevel.String() },
		},
		{
			name:        "created-tag",
			description: `Whether a tag was created. Either "true" or "false".`,
			value:       func(r *Result) string { return strconv.FormatBool(r.CreatedTag) },
		},
		{
			name:        "created-release",
			description: `Whether a release was created. Either "true" or "false".`,
			value:       func(r *Result) string { return strconv.FormatBool(r.CreatedRelease) },
		},
		{
			name:        "pre-release-hook-output",
			description: `*deprecated* Will be removed in a future release. Alias for pre-tag-hook-output`,
			value:       func(r *Result) string { return r.PrereleaseHookOutput },
		},
		{
			name:        "pre-release-hook-aborted",
			description: `*deprecated* Will be removed in a future release. Alias for pre-tag-hook-aborted`,
			value:       func(r *Result) string { return strconv.FormatBool(r.PrereleaseHookAborted) },
		},
		{
			name:        "pre-tag-hook-output",
			description: `The stdout of the pre-tag-hook. Empty if pre_release_hook is not set or if the hook returned an exit other than 0 or 10.`,
			value:       func(r *Result) string { return r.PreTagHookOutput },
		},
		{
			name:        "pre-tag-hook-aborted",
			description: `Whether pre-tag-hook issued an abort by exiting 10. Either "true" or "false".`,
			value:       func(r *Result) string { return strconv.FormatBool(r.PreTagHookAborted) },
		},
	}
}
