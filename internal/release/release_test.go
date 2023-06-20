package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v53/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train-action/v3/internal"
	"github.com/willabides/release-train-action/v3/internal/testutil"
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
git config user.name 'tester'
git config user.email 'tester'
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
git commit --allow-empty -m "second"
git tag second
git commit --allow-empty -m "third"
git tag v1.0.0
git tag third
echo 'module example.com/foo/v2' > src/go/go.mod
git add src/go/go.mod
git commit -m "fourth"
git tag v2.0.0
git tag foo
git commit --allow-empty -m "fifth"
git tag fifth
git commit --allow-empty -m "sixth"
git tag sixth
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
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.0.0", base)
				assert.Equal(t, repos.taggedCommits["head"], head)
				return []string{repos.taggedCommits["fourth"], repos.taggedCommits["head"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				switch sha {
				case repos.taggedCommits["fourth"]:
					return []internal.BasePull{{Number: 1, Labels: []string{"MinorAlias"}}}, nil
				case repos.taggedCommits["head"]:
					return []internal.BasePull{}, nil
				default:
					e := fmt.Errorf("unexpected sha %s", sha)
					t.Error(e)
					return nil, e
				}
			},
			StubCreateRelease: func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) (*github.RepositoryRelease, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.1.0", *opts.TagName)
				assert.Equal(t, "v2.1.0", *opts.Name)
				assert.Equal(t, "I got your release notes right here buddy\n", *opts.Body)
				assert.Equal(t, "legacy", *opts.MakeLatest)
				return &github.RepositoryRelease{
					ID:        github.Int64(1),
					UploadURL: github.String("localhost"),
				}, nil
			},
			StubUploadAsset: func(ctx context.Context, uploadURL, filename string, opts *github.UploadOptions) error {
				t.Helper()
				content, err := os.ReadFile(filename)
				if !assert.NoError(t, err) {
					return err
				}
				switch filepath.Base(filename) {
				case "foo.txt":
					assert.Equal(t, "foo\n", string(content))
				case "bar.txt":
					assert.Equal(t, "bar\n", string(content))
				default:
					e := fmt.Errorf("unexpected filename %s", filename)
					t.Error(e)
					return e
				}
				return nil
			},
			StubEditRelease: func(ctx context.Context, owner, repo string, id int64, opts *github.RepositoryRelease) error {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, int64(1), id)
				assert.Equal(t, false, *opts.Draft)
				return nil
			},
		}

		preHook := `
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

echo "I got your release notes right here buddy" >> "$RELEASE_NOTES_FILE"
echo "hello to my friends reading stdout"

echo foo > "$ASSETS_DIR/foo.txt"
echo bar > "$ASSETS_DIR/bar.txt"
`
		runner := Runner{
			CheckoutDir:    repos.clone,
			Ref:            "refs/tags/head",
			TagPrefix:      "v",
			Repo:           "orgName/repoName",
			PushRemote:     "origin",
			GithubClient:   &githubClient,
			CreateRelease:  true,
			PrereleaseHook: preHook,
			GithubToken:    "token",
			GoModFiles:     []string{"src/go/go.mod"},
			TempDir:        t.TempDir(),
			ReleaseRefs:    []string{"first", "fake", "sixth"},
			LabelAliases: map[string]string{
				"MINORALIAS": internal.LabelMinor,
			},
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:          "v2.0.0",
			PreviousVersion:      "2.0.0",
			FirstRelease:         false,
			ReleaseVersion:       semver.MustParse("2.1.0"),
			ReleaseTag:           "v2.1.0",
			ChangeLevel:          internal.ChangeLevelMinor,
			CreatedTag:           true,
			CreatedRelease:       true,
			PrereleaseHookOutput: "hello to my friends reading stdout\n",
		}, got)
		taggedSha, err := runCmd(repos.origin, nil, "git", "rev-parse", "v2.1.0")
		require.NoError(t, err)
		require.Equal(t, repos.taggedCommits["head"], taggedSha)
	})

	t.Run("first release", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, nil, "git", "checkout", "third")
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, repos.taggedCommits["first"], base)
				assert.Equal(t, repos.taggedCommits["third"], head)
				return []string{repos.taggedCommits["first"], repos.taggedCommits["third"]}, nil
			},
			StubGenerateReleaseNotes: func(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
				panic("GenerateReleaseNotes should not be called")
			},
			StubCreateRelease: func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) (*github.RepositoryRelease, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "x1.0.0", *opts.TagName)
				assert.Equal(t, "x1.0.0", *opts.Name)
				assert.Equal(t, "", *opts.Body)
				return &github.RepositoryRelease{
					ID:        github.Int64(1),
					UploadURL: github.String("localhost"),
				}, nil
			},
			StubEditRelease: func(ctx context.Context, owner, repo string, id int64, opts *github.RepositoryRelease) error {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, int64(1), id)
				assert.Equal(t, false, *opts.Draft)
				return nil
			},
		}
		runner := Runner{
			CheckoutDir:   repos.clone,
			Ref:           repos.taggedCommits["third"],
			TagPrefix:     "x",
			Repo:          "orgName/repoName",
			PushRemote:    "origin",
			GithubClient:  &githubClient,
			CreateRelease: true,
			InitialTag:    "x1.0.0",
			GoModFiles:    []string{"src/go/go.mod"},
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			FirstRelease:   true,
			ReleaseTag:     "x1.0.0",
			ReleaseVersion: semver.MustParse("1.0.0"),
			ChangeLevel:    internal.ChangeLevelNone,
			CreatedTag:     true,
			CreatedRelease: true,
		}, got)
	})

	t.Run("tags $RELEASE_TARGET", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)

		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.0.0", base)
				assert.Equal(t, repos.taggedCommits["head"], head)
				return []string{repos.taggedCommits["fourth"], repos.taggedCommits["head"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				switch sha {
				case repos.taggedCommits["fourth"]:
					return []internal.BasePull{{Number: 1, Labels: []string{internal.LabelMinor}}}, nil
				case repos.taggedCommits["head"]:
					return []internal.BasePull{}, nil
				default:
					e := fmt.Errorf("unexpected sha %s", sha)
					t.Error(e)
					return nil, e
				}
			},
		}
		preHook := `
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

git config user.name 'tester'
git config user.email 'tester'
echo foo > foo.txt
git add foo.txt > /dev/null
git commit -m "add foo.txt" > /dev/null
echo "$(git rev-parse HEAD)" > "$RELEASE_TARGET"
`
		runner := Runner{
			CheckoutDir:    repos.clone,
			Ref:            repos.taggedCommits["head"],
			TagPrefix:      "v",
			Repo:           "orgName/repoName",
			PushRemote:     "origin",
			GithubClient:   &githubClient,
			CreateTag:      true,
			PrereleaseHook: preHook,
			GithubToken:    "token",
			TempDir:        t.TempDir(),
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			FirstRelease:    false,
			ReleaseTag:      "v2.1.0",
			ReleaseVersion:  semver.MustParse("2.1.0"),
			PreviousVersion: "2.0.0",
			PreviousRef:     "v2.0.0",
			ChangeLevel:     internal.ChangeLevelMinor,
			CreatedTag:      true,
			CreatedRelease:  false,
		}, got)
		target := mustRunCmd(t, repos.origin, nil, "git", "rev-parse", "v2.1.0")
		// We don't know what the commit sha will be, but it should be different from head.
		require.NotEqual(t, repos.taggedCommits["head"], target)
	})

	t.Run("prerelease hook exits 10 to skip release", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)

		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.0.0", base)
				assert.Equal(t, repos.taggedCommits["head"], head)
				return []string{repos.taggedCommits["fourth"], repos.taggedCommits["head"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				switch sha {
				case repos.taggedCommits["fourth"]:
					return []internal.BasePull{{Number: 1, Labels: []string{internal.LabelMinor}}}, nil
				case repos.taggedCommits["head"]:
					return []internal.BasePull{}, nil
				default:
					e := fmt.Errorf("unexpected sha %s", sha)
					t.Error(e)
					return nil, e
				}
			},
		}
		preHook := `echo aborting; exit 10`
		runner := Runner{
			CheckoutDir:    repos.clone,
			Ref:            repos.taggedCommits["head"],
			TagPrefix:      "v",
			Repo:           "orgName/repoName",
			PushRemote:     "origin",
			GithubClient:   &githubClient,
			CreateTag:      true,
			PrereleaseHook: preHook,
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			FirstRelease:          false,
			ReleaseTag:            "v2.1.0",
			ReleaseVersion:        semver.MustParse("2.1.0"),
			PreviousVersion:       "2.0.0",
			PreviousRef:           "v2.0.0",
			ChangeLevel:           internal.ChangeLevelMinor,
			CreatedTag:            false,
			CreatedRelease:        false,
			PrereleaseHookOutput:  "aborting\n",
			PrereleaseHookAborted: true,
		}, got)
	})

	t.Run("generates release notes from API", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.0.0", base)
				assert.Equal(t, repos.taggedCommits["head"], head)
				return []string{repos.taggedCommits["v2.0.0"], repos.taggedCommits["head"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				return []internal.BasePull{
					{Number: 1, Labels: []string{internal.LabelMinor}},
				}, nil
			},
			StubGenerateReleaseNotes: func(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.1.0", opts.TagName)
				assert.Equal(t, "v2.0.0", *opts.PreviousTagName)
				return "release notes", nil
			},
			StubCreateRelease: func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) (*github.RepositoryRelease, error) {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, "v2.1.0", *opts.TagName)
				assert.Equal(t, "v2.1.0", *opts.Name)
				assert.Equal(t, "release notes", *opts.Body)
				return &github.RepositoryRelease{
					ID:        github.Int64(1),
					UploadURL: github.String("localhost"),
				}, nil
			},
			StubEditRelease: func(ctx context.Context, owner, repo string, id int64, opts *github.RepositoryRelease) error {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, int64(1), id)
				assert.Equal(t, false, *opts.Draft)
				return nil
			},
		}
		runner := Runner{
			CheckoutDir:   repos.clone,
			Ref:           repos.taggedCommits["head"],
			TagPrefix:     "v",
			Repo:          "orgName/repoName",
			PushRemote:    "origin",
			GithubClient:  &githubClient,
			CreateRelease: true,
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:     "v2.0.0",
			PreviousVersion: "2.0.0",
			FirstRelease:    false,
			ReleaseTag:      "v2.1.0",
			ReleaseVersion:  semver.MustParse("2.1.0"),
			ChangeLevel:     internal.ChangeLevelMinor,
			CreatedTag:      true,
			CreatedRelease:  true,
		}, got)
	})

	t.Run("shallow clone", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, nil, "git", "pull", "--depth=1")
		runner := &Runner{
			CheckoutDir: repos.clone,
		}
		_, err := runner.Run(ctx)
		require.EqualError(t, err, "shallow clones are not supported")
	})

	t.Run("not a git repo", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, nil, "rm", "-rf", ".git")
		runner := &Runner{
			CheckoutDir: repos.clone,
		}
		_, err := runner.Run(ctx)
		require.ErrorContains(t, err, "not a git repository")
	})

	t.Run("api error", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				return nil, errors.New("api error")
			},
		}
		_, err := (&Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["head"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			GithubClient: &githubClient,
		}).Run(ctx)
		require.EqualError(t, err, "api error")
	})

	t.Run("release error deletes tag", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				return []string{repos.taggedCommits["head"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				return []internal.BasePull{{Number: 2, Labels: []string{internal.LabelBreaking}}}, nil
			},
			StubGenerateReleaseNotes: func(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
				return "release notes", nil
			},
			StubCreateRelease: func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) (*github.RepositoryRelease, error) {
				return nil, errors.New("release error")
			},
		}
		runner := Runner{
			CheckoutDir:   repos.clone,
			Ref:           repos.taggedCommits["head"],
			TagPrefix:     "v",
			Repo:          "orgName/repoName",
			PushRemote:    "origin",
			GithubClient:  &githubClient,
			CreateRelease: true,
		}
		_, err := runner.Run(ctx)
		require.EqualError(t, err, "release error")
		ok, err := localTagExists(repos.origin, "v3.0.0")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("upload error deletes release", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		calledDelete := false
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				return []string{repos.taggedCommits["head"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				return []internal.BasePull{{Number: 2, Labels: []string{internal.LabelBreaking}}}, nil
			},
			StubGenerateReleaseNotes: func(ctx context.Context, owner, repo string, opts *github.GenerateNotesOptions) (string, error) {
				return "release notes", nil
			},
			StubCreateRelease: func(ctx context.Context, owner, repo string, opts *github.RepositoryRelease) (*github.RepositoryRelease, error) {
				return &github.RepositoryRelease{
					ID:        github.Int64(1),
					UploadURL: github.String("localhost"),
				}, nil
			},
			StubUploadAsset: func(ctx context.Context, uploadURL, filename string, opts *github.UploadOptions) error {
				return errors.New("upload error")
			},
			StubDeleteRelease: func(ctx context.Context, owner, repo string, id int64) error {
				t.Helper()
				assert.Equal(t, "orgName", owner)
				assert.Equal(t, "repoName", repo)
				assert.Equal(t, int64(1), id)
				calledDelete = true
				return nil
			},
		}
		preHook := `
#!/bin/sh
set -e

echo foo > "$ASSETS_DIR/foo.txt"
echo bar > "$ASSETS_DIR/bar.txt"
`
		runner := &Runner{
			CheckoutDir:    repos.clone,
			Ref:            repos.taggedCommits["head"],
			TagPrefix:      "v",
			Repo:           "orgName/repoName",
			PushRemote:     "origin",
			GithubClient:   &githubClient,
			CreateRelease:  true,
			PrereleaseHook: preHook,
			TempDir:        t.TempDir(),
		}
		_, err := runner.Run(ctx)
		require.ErrorContains(t, err, "upload error")
		require.Equal(t, true, calledDelete)
		ok, err := localTagExists(repos.origin, "v3.0.0")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("no create tag", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				return []string{repos.taggedCommits["head"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				return []internal.BasePull{{Number: 2, Labels: []string{internal.LabelBreaking}}}, nil
			},
		}
		got, err := (&Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["head"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			GithubClient: &githubClient,
		}).Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:     "v2.0.0",
			PreviousVersion: "2.0.0",
			FirstRelease:    false,
			ReleaseVersion:  semver.MustParse("3.0.0"),
			ReleaseTag:      "v3.0.0",
			ChangeLevel:     internal.ChangeLevelMajor,
		}, got)
	})

	t.Run("non-matching ref prevents tag", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				return []string{repos.taggedCommits["head"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				return []internal.BasePull{{Number: 2, Labels: []string{internal.LabelBreaking}}}, nil
			},
		}
		got, err := (&Runner{
			CheckoutDir:   repos.clone,
			Ref:           repos.taggedCommits["head"],
			TagPrefix:     "v",
			Repo:          "orgName/repoName",
			GithubClient:  &githubClient,
			CreateRelease: true,
			ReleaseRefs:   []string{"fake"},
		}).Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:     "v2.0.0",
			PreviousVersion: "2.0.0",
			FirstRelease:    false,
			ReleaseVersion:  semver.MustParse("3.0.0"),
			ReleaseTag:      "v3.0.0",
			ChangeLevel:     internal.ChangeLevelMajor,
		}, got)
	})

	t.Run("V0 prevents bumping to v1", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				return []string{repos.taggedCommits["second"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				return []internal.BasePull{{Number: 2, Labels: []string{internal.LabelBreaking}}}, nil
			},
		}
		got, err := (&Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["second"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			GithubClient: &githubClient,
			V0:           true,
		}).Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:     "v0.2.0",
			PreviousVersion: "0.2.0",
			FirstRelease:    false,
			ReleaseVersion:  semver.MustParse("0.3.0"),
			ReleaseTag:      "v0.3.0",
			ChangeLevel:     internal.ChangeLevelMinor,
		}, got)
	})

	t.Run("V0 errors when previous version is v1", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				return []string{repos.taggedCommits["third"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				return []internal.BasePull{{Number: 2, Labels: []string{internal.LabelMinor}}}, nil
			},
		}
		_, err := (&Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["third"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			GithubClient: &githubClient,
			V0:           true,
		}).Run(ctx)
		require.EqualError(t, err, `v0 flag is set, but previous version "1.0.0" has major version > 0`)
	})

	t.Run("iterates prerelease", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, nil, "git", "tag", "v2.1.0-rc.1", "fifth")
		githubClient := testutil.GithubStub{
			StubCompareCommits: func(ctx context.Context, owner, repo, base, head string) ([]string, error) {
				assert.Equal(t, "v2.1.0-rc.1", base)
				assert.Equal(t, repos.taggedCommits["head"], head)
				return []string{repos.taggedCommits["head"]}, nil
			},
			StubListPullRequestsWithCommit: func(ctx context.Context, owner, repo, sha string) ([]internal.BasePull, error) {
				return []internal.BasePull{{Number: 2, Labels: []string{internal.LabelMinor, internal.LabelPrerelease}}}, nil
			},
		}
		got, err := (&Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["head"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			GithubClient: &githubClient,
		}).Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:     "v2.1.0-rc.1",
			PreviousVersion: "2.1.0-rc.1",
			FirstRelease:    false,
			ReleaseVersion:  semver.MustParse("2.1.0-rc.2"),
			ReleaseTag:      "v2.1.0-rc.2",
			ChangeLevel:     internal.ChangeLevelMinor,
		}, got)
	})
}
