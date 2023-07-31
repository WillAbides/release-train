# GitHub Action Configuration

<!--- start action doc --->

## Inputs

### repo

default: `${{ github.repository }}`

GitHub repository in the form of owner/repo.

### check-pr

default: `${{ github.event.number }}`

Operates as if the given PR has already been merged. Useful for making sure the PR is properly labeled.
Skips tag and release.

### labels

PR label alias in the form of "<alias>=<label>" where <label> is a canonical label.

Accepts multiple values. One value per line.

### checkout-dir

default: `${{ github.workspace }}`

The directory where the repository is checked out.

### ref

default: `HEAD`

git ref.

### github-token

default: `${{ github.token }}`

The GitHub token to use for authentication. Must have `contents: write` permission if creating a release or tag.

### create-tag

Whether to create a tag for the release.

Only literal 'true' will be treated as true.

### create-release

Whether to create a release. Implies create-tag.

Only literal 'true' will be treated as true.

### draft

Leave the release as a draft.

Only literal 'true' will be treated as true.

### tag-prefix

default: `v`

The prefix to use for the tag.

### v0

Assert that current major version is 0 and treat breaking changes as minor changes.
Errors if the major version is not 0.

Only literal 'true' will be treated as true.

### initial-release-tag

default: `v0.0.0`

The tag to use if no previous version can be found. Set to "" to cause an error instead.

### pre-tag-hook

Command to run before tagging the release. You may abort the release by exiting with a non-zero exit code. Exit code 0
will continue the release. Exit code 10 will skip the release without error. Any other exit code will abort the release
with an error.

Environment variables available to the hook:

    RELEASE_VERSION
      The semantic version being released (e.g. 1.2.3).

    RELEASE_TAG
      The tag being created (e.g. v1.2.3).

    PREVIOUS_VERSION 
      The previous semantic version (e.g. 1.2.2). Empty on
      first release.

    FIRST_RELEASE
      Whether this is the first release. Either "true" or
      "false".

    GITHUB_TOKEN
      The GitHub token that was provided to release-train.

    RELEASE_NOTES_FILE
      A file path where you can write custom release notes.
      When nothing is written to this file, release-train
      will use GitHub's default release notes.

    RELEASE_TARGET
      A file path where you can write an alternate git ref
      to release instead of HEAD.

    ASSETS_DIR
	  A directory where you can write release assets. All
	  files in this directory will be uploaded as release
	  assets.

In addition to the above environment variables, all variables from release-train's environment are available to the
hook.

When the hook creates a tag named $RELEASE_TAG, it will be used as the release target instead of either HEAD or the
value written to $RELEASE_TARGET.

### pre-release-hook

__Deprecated__ - deprecated

*deprecated* Will be removed in a future release. Alias for pre-tag-hook.

### release-refs

Only allow tags and releases to be created from matching refs. Refs can be patterns accepted by git-show-ref.
If undefined, any branch can be used.

Accepts multiple values. One value per line.

### tempdir

The prefix to use with mktemp to create a temporary directory.

### debug

Enable debug logging.

Only literal 'true' will be treated as true.

### release-train-bin

Path to release-train binary. Only needed if you're using a custom release-train binary.

## Outputs

### previous-ref

A git ref pointing to the previous release, or the current ref if no previous release can be found.

### previous-version

The previous version on the release branch.

### first-release

Whether this is the first release on the release branch. Either "true" or "false".

### release-version

The version of the new release. Empty if no release is called for.

### release-tag

The tag of the new release. Empty if no release is called for.

### change-level

The level of change in the release. Either "major", "minor", "patch" or "none".

### created-tag

Whether a tag was created. Either "true" or "false".

### created-release

Whether a release was created. Either "true" or "false".

### pre-release-hook-output

*deprecated* Will be removed in a future release. Alias for pre-tag-hook-output

### pre-release-hook-aborted

*deprecated* Will be removed in a future release. Alias for pre-tag-hook-aborted

### pre-tag-hook-output

The stdout of the pre-tag-hook. Empty if pre_release_hook is not set or if the hook returned an exit other than 0 or 10.

### pre-tag-hook-aborted

Whether pre-tag-hook issued an abort by exiting 10. Either "true" or "false".
<!--- end action doc --->
