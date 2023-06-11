# Contributing to release-train

## Scripts

release-train uses a number of scripts to automate common tasks. They are found in the
`script` directory.

<!--- start script descriptions --->

### bindown

script/bindown runs bindown

### cibuild

script/cibuild is run by CI to test this project. It can also be run locally.

### fmt

script/fmt formats go code and shell scripts.

### generate

script/generate runs all generators for this repo.
`script/generate --check` checks that the generated files are up to date.

### lint

script/lint runs linters.

### release-train

script/release-train builds and runs release-train.

### test

script/test runs tests.

### update-docs

script/update-docs updates README.md with a description of the action.

<!--- end script descriptions --->

## Releasing

Releases are automated with GitHub Actions. The release workflow runs on every push to main and determines the version
to release based on the labels of the PRs that have been merged since the last release. The labels it looks for are:

| Label           | Change Level |
|-----------------|--------------|
| breaking        | major        |
| breaking change | major        |
| major           | major        |
| semver:major    | major        |
| bug             | patch        |
| enhancement     | minor        |
| minor           | minor        |
| semver:minor    | minor        |
| bug             | patch        |
| patch           | patch        |
| semver:patch    | patch        |
