package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-github/v53/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustRunCmd(t *testing.T, dir string, env map[string]string, name string, args ...string) string {
	t.Helper()
	out, err := runCmd(dir, env, name, args...)
	require.NoError(t, err)
	return out
}

func Test_releaseRunner_run(t *testing.T) {
	t.Parallel()
	type gitRepos struct {
		origin        string
		clone         string
		taggedCommits map[string]string
	}
	setupGit := func(t *testing.T) *gitRepos {
		t.Helper()
		originDir := t.TempDir()
		cloneDir := t.TempDir()
		mustRunCmd(t, originDir, nil, "sh", "-c", `
git init
mkdir -p src/go
echo 'module example.com/foo' > src/go/go.mod
git add src/go/go.mod
git commit -am "first"
git tag first
git tag v0.1.0
git tag foo0.1.0
git tag v0.1.1
git tag foo0.1.1
git tag v0.2.0
git tag foo0.2.0
git tag v1.0.0
git commit --allow-empty -m "second"
git tag second
echo 'module example.com/foo/v2' > src/go/go.mod
git add src/go/go.mod
git commit -m "third"
git tag v2.0.0
git tag v2.0.1-rc1
git tag foo
git commit --allow-empty -m "fourth"
git tag fourth
git commit --allow-empty -m "fifth"
git tag head
`,
		)
		tags := mustRunCmd(t, originDir, nil, "git", "tag", "-l")
		tags = strings.ReplaceAll(tags, "\r\n", "\n")
		taggedCommits := map[string]string{}
		for _, tag := range strings.Split(tags, "\n") {
			taggedCommits[tag] = mustRunCmd(t, originDir, nil, "git", "rev-parse", tag)
		}
		mustRunCmd(t, cloneDir, nil, "git", "clone", originDir, ".")
		return &gitRepos{
			origin:        originDir,
			clone:         cloneDir,
			taggedCommits: taggedCommits,
		}
	}

	t.Run("kitchen sink", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.0.0", base)
				assert.Equal(t, repos.taggedCommits["head"], head)
				return []string{repos.taggedCommits["fourth"], repos.taggedCommits["head"]}, nil
			},
			listPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]nextResultPull, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				switch sha {
				case repos.taggedCommits["fourth"]:
					return []nextResultPull{{Number: 1, Labels: []string{"semver:minor"}}}, nil
				case repos.taggedCommits["head"]:
					return []nextResultPull{}, nil
				default:
					e := fmt.Errorf("unexpected sha %s", sha)
					t.Error(e)
					return nil, e
				}
			},
			createRelease: func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) error {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.1.0", *opts.TagName)
				assert.Equal(t, "v2.1.0", *opts.Name)
				assert.Equal(t, "I got your release notes right here buddy\n", *opts.Body)
				assert.Equal(t, "legacy", *opts.MakeLatest)
				return nil
			},
		}

		postHook := `
#!/bin/sh
set -e

assertVar() {
  name="$1"
  want="$2"
  got="$3"
  if [ "$want" != "$got" ]; then
    echo "$name was '$got' wanted '$want'" >&2
    exit 1
  fi
}

assertVar RELEASE_VERSION 2.1.0 "$RELEASE_VERSION"
assertVar RELEASE_TAG v2.1.0 "$RELEASE_TAG"
assertVar PREVIOUS_VERSION 2.0.0 "$PREVIOUS_VERSION"
assertVar FIRST_RELEASE false "$FIRST_RELEASE"
assertVar GITHUB_TOKEN token "$GITHUB_TOKEN"
`
		preHook := postHook + `
echo "I got your release notes right here buddy" >> "$RELEASE_NOTES_FILE"
`
		runner := releaseRunner{
			checkoutDir:     repos.clone,
			ref:             repos.taggedCommits["head"],
			tagPrefix:       "v",
			repo:            "orgName/repoName",
			pushRemote:      "origin",
			githubClient:    &githubClient,
			createRelease:   true,
			prereleaseHook:  preHook,
			postreleaseHook: postHook,
			githubToken:     "token",
			goModFiles:      []string{"src/go/go.mod"},
		}
		got, err := runner.run(ctx)
		require.NoError(t, err)
		require.Equal(t, &releaseResult{
			PreviousRef:     "v2.0.0",
			PreviousVersion: "2.0.0",
			FirstRelease:    false,
			ReleaseVersion:  "2.1.0",
			ReleaseTag:      "v2.1.0",
		}, got)
		taggedSha, err := runCmd(repos.origin, nil, "git", "rev-parse", "v2.1.0")
		require.NoError(t, err)
		require.Equal(t, repos.taggedCommits["head"], taggedSha)
	})

	t.Run("first release", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, nil, "git", "checkout", "second")
		githubClient := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, repos.taggedCommits["first"], base)
				assert.Equal(t, repos.taggedCommits["second"], head)
				return []string{repos.taggedCommits["first"], repos.taggedCommits["second"]}, nil
			},
			generateReleaseNotes: func(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
				panic("GenerateReleaseNotes should not be called")
			},
			createRelease: func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) error {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "x1.0.0", *opts.TagName)
				assert.Equal(t, "x1.0.0", *opts.Name)
				assert.Equal(t, "", *opts.Body)
				return nil
			},
		}
		runner := releaseRunner{
			checkoutDir:   repos.clone,
			ref:           repos.taggedCommits["second"],
			tagPrefix:     "x",
			repo:          "orgName/repoName",
			pushRemote:    "origin",
			githubClient:  &githubClient,
			createRelease: true,
			initialTag:    "x1.0.0",
			goModFiles:    []string{"src/go/go.mod"},
		}
		got, err := runner.run(ctx)
		require.NoError(t, err)
		require.Equal(t, &releaseResult{
			FirstRelease:   true,
			ReleaseTag:     "x1.0.0",
			ReleaseVersion: "1.0.0",
		}, got)
	})

	t.Run("generates release notes from API", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.0.0", base)
				assert.Equal(t, repos.taggedCommits["head"], head)
				return []string{repos.taggedCommits["v2.0.0"], repos.taggedCommits["head"]}, nil
			},
			listPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]nextResultPull, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				return []nextResultPull{
					{Number: 1, Labels: []string{"semver:minor"}},
				}, nil
			},
			generateReleaseNotes: func(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.1.0", opts.TagName)
				assert.Equal(t, "v2.0.0", *opts.PreviousTagName)
				return "release notes", nil
			},
			createRelease: func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) error {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.1.0", *opts.TagName)
				assert.Equal(t, "v2.1.0", *opts.Name)
				assert.Equal(t, "release notes", *opts.Body)
				return nil
			},
		}
		runner := releaseRunner{
			checkoutDir:   repos.clone,
			ref:           repos.taggedCommits["head"],
			tagPrefix:     "v",
			repo:          "orgName/repoName",
			pushRemote:    "origin",
			githubClient:  &githubClient,
			createRelease: true,
		}
		got, err := runner.run(ctx)
		require.NoError(t, err)
		require.Equal(t, &releaseResult{
			PreviousRef:     "v2.0.0",
			PreviousVersion: "2.0.0",
			FirstRelease:    false,
			ReleaseTag:      "v2.1.0",
			ReleaseVersion:  "2.1.0",
		}, got)
	})

	t.Run("shallow clone", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, nil, "git", "pull", "--depth=1")
		runner := &releaseRunner{
			checkoutDir: repos.clone,
		}
		_, err := runner.run(ctx)
		require.EqualError(t, err, "shallow clones are not supported")
	})

	t.Run("not a git repo", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, nil, "rm", "-rf", ".git")
		runner := &releaseRunner{
			checkoutDir: repos.clone,
		}
		_, err := runner.run(ctx)
		require.ErrorContains(t, err, "not a git repository")
	})

	t.Run("api error", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				return nil, errors.New("api error")
			},
		}
		_, err := (&releaseRunner{
			checkoutDir:  repos.clone,
			ref:          repos.taggedCommits["head"],
			tagPrefix:    "v",
			repo:         "orgName/repoName",
			githubClient: &githubClient,
		}).run(ctx)
		require.EqualError(t, err, "api error")
	})

	t.Run("no create tag", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := wrapperStub{
			compareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				return []string{repos.taggedCommits["head"]}, nil
			},
			listPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]nextResultPull, error) {
				return []nextResultPull{{Number: 2, Labels: []string{"semver:major"}}}, nil
			},
		}
		got, err := (&releaseRunner{
			checkoutDir:  repos.clone,
			ref:          repos.taggedCommits["head"],
			tagPrefix:    "v",
			repo:         "orgName/repoName",
			githubClient: &githubClient,
		}).run(ctx)
		require.NoError(t, err)
		require.Equal(t, &releaseResult{
			PreviousRef:     "v2.0.0",
			PreviousVersion: "2.0.0",
			FirstRelease:    false,
			ReleaseVersion:  "3.0.0",
			ReleaseTag:      "v3.0.0",
		}, got)
	})
}
