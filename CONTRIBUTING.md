# Contributing to release-train

Your issues and pull requests are welcome. If you have a non-trivial change, you
may want to open an issue first so we can discuss whether it will be a good fit
before you spend time on it. Don't let that stop you from coding, though. Just
be aware that it may not get merged into this repo.

## Scripts

release-train uses a number of scripts to automate common tasks. They are found in the
`script` directory.

<!--- start script descriptions --->

### bindown

script/bindown runs bindown

### bindown-template

script/bindown-template builds a bindown template for release-train.
Usage: script/bindown-template <release> <output-file>

### bootstrap-bindown.sh

bootstraps bindown -- only used by script/bindown

### check-module-version

script/check-module-version checks that this module's name works with the given version.
Usage: script/check-module-version <version>

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

script/release-train builds and runs release-train. When run from a github action, it will attempt
to download the version of release-train configured in the action instead of building it.

### test

script/test runs tests.

### update-docs

script/update-docs updates README.md with a description of the action.

<!--- end script descriptions --->
