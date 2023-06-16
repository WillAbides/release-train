package releasetrain

import (
	"os"

	"github.com/alecthomas/kong"
	"github.com/willabides/release-train-action/v2/internal/action"
	"gopkg.in/yaml.v3"
)

const actionBoolSuffix = "Only literal 'true' will be treated as true."

type actionCmd struct{}

func (cmd *actionCmd) Run(k *kong.Context) error {
	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	k.Model.Vars()
	return enc.Encode(getAction(k))
}

func getAction(k *kong.Context) *action.CompositeAction {
	vars := k.Model.Vars()
	thisAction := &action.CompositeAction{
		Name:        "release-train",
		Description: "hop on the release train",
		Branding: &action.Branding{
			Icon:  "send",
			Color: "yellow",
		},
		Inputs: action.NewOrderedMap(
			action.MapEntry("check_pr_labels", action.Input{
				Description: `Instead of releasing, check that the PR has a label indicating the type of change.` +
					"\n\n" + actionBoolSuffix,
				Default: "${{ github.event_name == 'pull_request' }}",
			}),

			action.MapEntry("checkout_dir", action.Input{
				Description: vars["checkout_dir_help"],
				Default:     "${{ github.workspace }}",
			}),

			action.MapEntry("ref", action.Input{
				Description: vars["ref_help"],
				Default:     "${{ github.ref }}",
			}),

			action.MapEntry("github_token", action.Input{
				Description: vars["github_token_help"],
				Default:     "${{ github.token }}",
			}),

			action.MapEntry("create_tag", action.Input{
				Description: vars["create_tag_help"] + "\n\n" + actionBoolSuffix,
			}),

			action.MapEntry("create_release", action.Input{
				Description: vars["create_release_help"] + "\n\n" + actionBoolSuffix,
			}),

			action.MapEntry("tag_prefix", action.Input{
				Description: vars["tag_prefix_help"],
				Default:     vars["tag_prefix_default"],
			}),

			action.MapEntry("v0", action.Input{
				Description: vars["v0_help"] + "\n\n" + actionBoolSuffix,
			}),

			action.MapEntry("initial_release_tag", action.Input{
				Description: vars["initial_tag_help"],
				Default:     vars["initial_tag_default"],
			}),

			action.MapEntry("pre_release_hook", action.Input{
				Description: vars["pre_release_hook_help"],
			}),

			action.MapEntry("validate_go_module", action.Input{
				Description: vars["go_mod_file_help"],
			}),

			action.MapEntry("no_release", action.Input{
				Description: `
If set to true, this will be a no-op. This is useful for creating a new repository or branch that isn't ready for
release yet.` + "\n\n" + actionBoolSuffix,
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
					Run: `"${{ github.action_path }}"/action/check_pr_labels`,
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
					Run: `"${{ github.action_path }}"/action/release`,
				},
			},
		},
	}
	return thisAction
}
