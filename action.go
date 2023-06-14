package main

import (
	"github.com/willabides/release-train-action/v2/internal/action"
)

const (
	createTagHelp     = `Whether to create a tag for the release.`
	createReleaseHelp = `Whether to create a release.`
	validateGoModHelp = `
Validates that the name of the go module at the given path matches the major version of the release. For example,
validation will fail when releasing v3.0.0 when the module name is ` + "`my_go_module/v2`."
)

var thisAction = &action.CompositeAction{
	Name:        "release-train",
	Description: "hop on the release train",
	Branding: &action.Branding{
		Icon:  "send",
		Color: "yellow",
	},
	Inputs: action.NewOrderedMap(
		action.MapEntry("check_pr_labels", action.Input{
			Description: `
Instead of releasing, check that the PR has a label indicating the type of change.  

Only literal 'true' will be treated as true.
`,
			Default: "${{ github.event_name == 'pull_request' }}",
		}),

		action.MapEntry("checkout_dir", action.Input{
			Description: `
The directory where the repository is checked out.
`,
			Default: "${{ github.workspace }}",
		}),

		action.MapEntry("ref", action.Input{
			Description: `
The branch or tag to release.
`,
			Default: "${{ github.ref }}",
		}),

		action.MapEntry("github_token", action.Input{
			Description: `
The GitHub token to use for authentication. Must have ` + "`contents: write`" + ` permission if creating a release or tag.
`,
			Default: "${{ github.token }}",
		}),

		action.MapEntry("create_tag", action.Input{
			Description: createTagHelp + "  Only literal 'true' will be treated as true.",
		}),

		action.MapEntry("create_release", action.Input{
			Description: createReleaseHelp + "  Only literal 'true' will be treated as true.  Implies create_tag.",
		}),

		action.MapEntry("tag_prefix", action.Input{
			Description: `
The prefix to use for the tag.`,
			Default: "v",
		}),

		action.MapEntry("initial_release_tag", action.Input{
			Description: `
The tag to use if no previous version can be found.

Set to empty string to disable cause it to error if no previous version can be found.
`,
			Default: "v0.0.0",
		}),

		action.MapEntry("pre_release_hook", action.Input{
			Description: `
Command to run before creating the release. You may abort the release by exiting with a non-zero exit code.

Exit code 0 will continue the release. Exit code 10 will skip the release without error. Any other exit code will
abort the release with an error.

You may provide custom release notes by writing to the file at ` + "`$RELEASE_NOTES_FILE`" + `:

` + "```" + `
  echo "my release notes" > "$RELEASE_NOTES_FILE"
` + "```" + `

You can update the git ref to be released by writing it to the file at ` + "`$RELEASE_TARGET`" + `:

` + "```" + `
  # ... update some files ...
  git commit -am "prepare release $RELEASE_TAG"
  echo "$(git rev-parse HEAD)" > "$RELEASE_TARGET"
` + "```" + `

The environment variables ` + "`RELEASE_VERSION`" + `, ` + "`RELEASE_TAG`" + `, ` + "`PREVIOUS_VERSION`" + `, ` + "`FIRST_RELEASE`" + `, ` + "`GITHUB_TOKEN`" + `,
` + "`RELEASE_NOTES_FILE`" + ` and ` + "`RELEASE_TARGET`" + ` will be set.
`,
		}),

		action.MapEntry("post_release_hook", action.Input{
			Description: `
Command to run after the release is complete. This is useful for adding artifacts to your release.

The environment variables ` + "`RELEASE_VERSION`" + `, ` + "`RELEASE_TAG`" + `, ` + "`PREVIOUS_VERSION`" + `, ` + "`FIRST_RELEASE`" + ` and ` + "`GITHUB_TOKEN`" + `
will be set.
`,
		}),

		action.MapEntry("validate_go_module", action.Input{
			Description: validateGoModHelp,
		}),

		action.MapEntry("no_release", action.Input{
			Description: `
If set to true, this will be a no-op. This is useful for creating a new repository or branch that isn't ready for
release yet.

Only literal 'true' will be treated as true.
`,
		}),
	),

	Outputs: action.NewOrderedMap(
		action.MapEntry("previous_ref", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.previous_ref }}",
			Description: "A git ref pointing to the previous release, or the current ref if no previous release can be found.",
		}),

		action.MapEntry("previous_version", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.previous_version }}",
			Description: "The previous version on the release branch.",
		}),

		action.MapEntry("first_release", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.first_release }}",
			Description: "Whether this is the first release on the release branch. Either \"true\" or \"false\".",
		}),

		action.MapEntry("release_version", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.release_version }}",
			Description: "The version of the new release. Empty if no release is called for.",
		}),

		action.MapEntry("release_tag", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.release_tag }}",
			Description: "The tag of the new release. Empty if no release is called for.",
		}),

		action.MapEntry("change_level", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.change_level }}",
			Description: "The level of change in the release. Either \"major\", \"minor\", \"patch\" or \"no change\".",
		}),

		action.MapEntry("created_tag", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.created_tag }}",
			Description: "Whether a tag was created. Either \"true\" or \"false\".",
		}),

		action.MapEntry("created_release", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.created_release }}",
			Description: "Whether a release was created. Either \"true\" or \"false\".",
		}),

		action.MapEntry("pre_release_hook_output", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.pre_release_hook_output }}",
			Description: "The stdout of the pre_release_hook. Empty if pre_release_hook is not set or if the hook returned an exit other than 0 or 10.",
		}),

		action.MapEntry("pre_release_hook_aborted", action.CompositeOutput{
			Value:       "${{ steps.release.outputs.pre_release_hook_aborted }}",
			Description: "Whether pre_release_hook issued an abort by exiting 10. Either \"true\" or \"false\".",
		}),
	),

	Runs: action.CompositeRuns{
		Using: "composite",
		Steps: []action.CompositeStep{
			{
				Id:               "check_pr_labels",
				If:               "${{ inputs.check_pr_labels == 'true' }}",
				Shell:            "sh",
				WorkingDirectory: "${{ inputs.checkout_dir }}",
				Env: action.NewOrderedMap(
					action.MapEntry("GITHUB_TOKEN", "${{ inputs.github_token }}"),
					action.MapEntry("GH_TOKEN", "${{ inputs.github_token }}"),
					action.MapEntry("NO_RELEASE", "${{ inputs.no_release }}"),
				),
				Run: `"${{ github.action_path }}"/src/check_pr_labels`,
			},
			{
				Id:               "release",
				If:               "${{ inputs.check_pr_labels != 'true' }}",
				Shell:            "sh",
				WorkingDirectory: "${{ inputs.checkout_dir }}",
				Env: action.NewOrderedMap(
					action.MapEntry("REF", "${{ inputs.ref }}"),
					action.MapEntry("GITHUB_TOKEN", "${{ inputs.github_token }}"),
					action.MapEntry("CREATE_TAG", "${{ inputs.create_tag }}"),
					action.MapEntry("CREATE_RELEASE", "${{ inputs.create_release }}"),
					action.MapEntry("TAG_PREFIX", "${{ inputs.tag_prefix }}"),
					action.MapEntry("INITIAL_RELEASE_TAG", "${{ inputs.initial_release_tag }}"),
					action.MapEntry("PRE_RELEASE_HOOK", "${{ inputs.pre_release_hook }}"),
					action.MapEntry("POST_RELEASE_HOOK", "${{ inputs.post_release_hook }}"),
					action.MapEntry("VALIDATE_GO_MODULE", "${{ inputs.validate_go_module }}"),
					action.MapEntry("NO_RELEASE", "${{ inputs.no_release }}"),
					action.MapEntry("GITHUB_REPOSITORY", "${{ github.repository }}"),
				),
				Run: `"${{ github.action_path }}"/src/release`,
			},
		},
	},
}
