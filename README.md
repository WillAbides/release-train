# release-train

[Contributions welcome](./CONTRIBUTING.md).

**release-train** creates releases for every PR merge in your repository. No
magic commit message is required. You only need to label your PRs with the
appropriate labels and run release-train -- either from the command line or a
GitHub Action.

Release-train is inspired
by [semantic-release](https://github.com/semantic-release/semantic-release) and
has a few advantages for somebody with my biases and proclivities:

- It doesn't require special commit messages, so you won't need to squash
  commits or ask contributors to rebase before merging. You are free to follow
  whatever commit conventions suit you,
  including [xkcd commit conventions](https://xkcd.com/1296/).
- No need for npm or other package managers.
- No plugin configuration. Release-train has no plugins. You can do anything a
  plugin would do from the pre-tag hook.

## Labels

Labels and label aliases are **not** case-sensitive

Release-train uses pull request labels to determine the change level for each
PR. The release updates the version based on the highest change level found in
the PRs being released -- typically only one PR if you are doing continuous
releases.

| Label             | Effect                  | Example              |
|-------------------|-------------------------|----------------------|
| `semver:breaking` | Increment major version | `v0.1.2` -> `v1.0.0` |
| `semver:minor`    | Increment minor version | `v0.1.2` -> `v0.2.0` |
| `semver:patch`    | Increment patch version | `v0.1.2` -> `v0.1.3` |
| `semver:none`     | No version change       | `v0.1.2` -> `v0.1.2` |

### Prerelease

In addition there are prerelease and stable labels used to determine whether to
publish a prerelease or stable release. These are `semver:prerelease` and
`semver:stable`.

`semver:prerelease` must be combined with version labels to determine what the
stable portion of the prerelease version will be.

`semver:prerelease` can also specify the prerelease identifier. For example,
`semver:prerelease:alpha` will create a prerelease with the identifier `alpha`.
A prerelease cannot contain PRs with conflicting identifiers.

| Previous release | Labels                                       | Next release     | Notes                                               |
|------------------|----------------------------------------------|------------------|-----------------------------------------------------|   
| `v0.1.2`         | `semver:breaking`, `semver:prerelease`       | `v1.0.0-0`       |                                                     |
| `v0.1.2`         | `semver:breaking`, `semver:prerelease:alpha` | `v1.0.0-alpha.0` |                                                     |
| `v1.0.0-0`       | `semver:breaking`, `semver:prerelease`       | `v1.0.0-1`       | Doesn't iterate major because minor and patch are 0 |
| `v1.1.0-0`       | `semver:breaking`, `semver:prerelease`       | `v2.0.0-0`       | Iterates major because minor is not 0               |
| `v1.0.0-2`       | `semver:breaking`, `semver:prerelease:alpha` | `v1.0.0-alpha.0` | New identifier resets the prerelease iterator       |

When the most recent release is a prerelease, `semver:stable` is used to
indicate that the next release should be stable. When the most recent release is
a prerelease and one PR in the next release is labeled `semver:stable`, then all
PRs in the next release must be labeled `semver:stable`.

If `semver:stable` is combined with a version label, the version is incremented
**after** making the release stable. For instance, if the latest release
is `v1.0.0-3` and you merge a PR labeled `semver:stable`, the next release will
be `v1.0.0`, but if the PR is labeled both `semver:stable`
and `semver:minor`, the next release will be `v1.1.0`.

### Label Aliases

The labels listed above are the canonical labels, but you can use aliases that
are better suited to your project.

For example if you want something close
to [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) your
github action config might contain something like this:

```yaml
labels: |
  "breaking change"=semver:breaking
  fix=semver:patch
  feat=semver:minor
  perf=semver:patch
  chore=semver:none
  docs=semver:none
  style=semver:none
  refactor=semver:patch
```

## Release steps

When run from a release branch, release-train follows these steps to publish a
release. Some options such as `--check-pr` will modify this behavior.

1. **Find the previous release tag** by searching backward through git history
   for the first tag formatted like `<prefix><semantic version>`. When no
   previous tag is found it uses `v0.0.0` or the value from `--initial-tag`.
2. **Find the next release version** by looking at the diff between HEAD and the
   previous release tag and inspecting the pull request where each commit was
   introduced. The previous version is incremented by the highest change level
   found.
3. **Run pre-tag-hook**. This is where you can do things like validate the
   release, built release artifacts or generate a changelog.
4. **Create and push the new git tag** if `--create-tag` is set.
5. **Create a draft release** if `--create-release` is set. It starts as a draft
   to avoid publishing a release that doesn't have all the necessary artifacts
   yet.
6. **Upload release assets**. Any files written to `$ASSETS_DIR` will be
   uploaded as release assets.
7. **Publish the release**.
8. **Emit output** including release version, tag, change level, etc that you
   use in notifications or other actions.

## Pre-tag hook

The pre-tag hook is a shell script that runs before the new release is
tagged. It lets you do some customizations like creating release notes, building
release artifacts, or validating the release.

See [the action doc](./doc/action.md#pre-tag-hook) for more details.

## GitHub Action Configuration

See [action.md](./doc/action.md).

## Command Line

### Installation

#### Install with [bindown](https://github.com/WillAbides/bindown)

```shell
bindown template-source add release-train https://github.com/WillAbides/release-train/releases/latest/download/bindown.yaml
bindown dependency add release-train --source release-train
```

#### Install from go source

```shell
go install github.com/willabides/release-train/v3@latest
```

#### Download a release

Pick a release from
the [releases page](https://github.com/WillAbides/release-train/releases) and
download the appropriate binary for your platform.

### Usage

<!--- start usage output --->

```
Usage: release-train

Release every PR merge. No magic commit message required.

Flags:
  -h, --help                          Show context-sensitive help.
      --version
      --repo=STRING                   GitHub repository in the form of owner/repo.
      --check-pr=INT                  Operates as if the given PR has already been merged. Useful
                                      for making sure the PR is properly labeled. Skips tag and
                                      release.
      --label=<alias>=<label>;...     PR label alias in the form of "<alias>=<label>" where <label>
                                      is a canonical label.
  -C, --checkout-dir="."              The directory where the repository is checked out.
      --ref="HEAD"                    git ref.
      --create-tag                    Whether to create a tag for the release.
      --create-release                Whether to create a release. Implies create-tag.
      --draft                         Leave the release as a draft.
      --tag-prefix="v"                The prefix to use for the tag.
      --v0                            Assert that current major version is 0 and treat breaking
                                      changes as minor changes. Errors if the major version is not
                                      0.
      --initial-tag="v0.0.0"          The tag to use if no previous version can be found. Set to ""
                                      to cause an error instead.
      --pre-tag-hook=<command>        Command to run before tagging the release. You may abort the
                                      release by exiting with a non-zero exit code. Exit code 0
                                      will continue the release. Exit code 10 will skip the release
                                      without error. Any other exit code will abort the release with
                                      an error.

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

                                      In addition to the above environment variables, all variables
                                      from release-train's environment are available to the hook.

                                      When the hook creates a tag named $RELEASE_TAG, it will be
                                      used as the release target instead of either HEAD or the value
                                      written to $RELEASE_TARGET.
      --pre-release-hook=<command>    *deprecated* Will be removed in a future release. Alias for
                                      pre-tag-hook.
      --release-ref=<branch>,...      Only allow tags and releases to be created from matching refs.
                                      Refs can be patterns accepted by git-show-ref. If undefined,
                                      any branch can be used.
      --push-remote="origin"          The remote to push tags to.
      --tempdir=STRING                The prefix to use with mktemp to create a temporary directory.
      --github-api-url="https://api.github.com"
                                      GitHub API URL.
      --output-format="json"          Output either json our GitHub action output.
      --debug                         Enable debug logging.
```

<!--- end usage output --->
