package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/willabides/release-train/v3/internal/github"
	"github.com/willabides/release-train/v3/internal/mocks"
	"go.uber.org/mock/gomock"
)

func mustRunCmd(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	out, err := runCmd(t.Context(), &runCmdOpts{dir: dir}, name, args...)
	require.NoError(t, err)
	return out
}

func Test_releaseRunner_run(t *testing.T) {
	t.Parallel()
	mergeSha := "4aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	type gitRepos struct {
		origin        string
		clone         string
		taggedCommits map[string]string
	}
	setupGit := func(t *testing.T) *gitRepos {
		t.Helper()
		originDir := t.TempDir()
		cloneDir := t.TempDir()
		mustRunCmd(t, originDir, "sh", "-c", `
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
`)
		tags := mustRunCmd(t, originDir, "git", "tag", "-l")
		tags = strings.ReplaceAll(tags, "\r\n", "\n")
		taggedCommits := map[string]string{}
		for _, tag := range strings.Split(tags, "\n") {
			taggedCommits[tag] = mustRunCmd(t, originDir, "git", "rev-parse", tag)
		}
		mustRunCmd(t, cloneDir, "git", "clone", originDir, ".")
		return &gitRepos{
			origin:        originDir,
			clone:         cloneDir,
			taggedCommits: taggedCommits,
		}
	}

	t.Run("kitchen sink", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 2,
				Commits: []string{repos.taggedCommits["fourth"], repos.taggedCommits["head"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 1}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["fourth"]).Return(
			[]github.BasePull{{Number: 1, MergeCommitSha: mergeSha, Labels: []string{"MinorAlias"}}}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["head"]).Return(
			[]github.BasePull{}, nil,
		)
		githubClient.EXPECT().CreateRelease(gomock.Any(), "orgName", "repoName", "v2.1.0", "I got your release notes right here buddy\n", false).Return(
			&github.RepoRelease{
				ID:        1,
				UploadURL: "localhost",
			}, nil,
		)
		githubClient.EXPECT().UploadAsset(gomock.Any(), "localhost", gomock.Any()).DoAndReturn(
			func(_ context.Context, _, filename string) error {
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
		).AnyTimes()
		githubClient.EXPECT().PublishRelease(gomock.Any(), "orgName", "repoName", "true", int64(1)).Return(nil)

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
assertVar PREVIOUS_REF v2.0.0 "$PREVIOUS_REF"
assertVar PREVIOUS_STABLE_VERSION 2.0.0 "$PREVIOUS_STABLE_VERSION"
assertVar PREVIOUS_STABLE_REF v2.0.0 "$PREVIOUS_STABLE_REF"
assertVar FIRST_RELEASE false "$FIRST_RELEASE"
assertVar GITHUB_TOKEN token "$GITHUB_TOKEN"

echo "I got your release notes right here buddy" >> "$RELEASE_NOTES_FILE"
echo "hello to my friends reading stdout"

echo foo > "$ASSETS_DIR/foo.txt"
echo bar > "$ASSETS_DIR/bar.txt"
`
		runner := Runner{
			CheckoutDir:   repos.clone,
			Ref:           "refs/tags/head",
			TagPrefix:     "v",
			Repo:          "orgName/repoName",
			PushRemote:    "origin",
			GithubClient:  githubClient,
			CreateRelease: true,
			PreTagHook:    preHook,
			GithubToken:   "token",
			TempDir:       t.TempDir(),
			ReleaseRefs:   []string{"first", "fake", "sixth"},
			LabelAliases: map[string]string{
				"MINORALIAS": labelMinor,
			},
			MakeLatest: "true",
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:           "v2.0.0",
			PreviousVersion:       "2.0.0",
			PreviousStableRef:     "v2.0.0",
			PreviousStableVersion: "2.0.0",
			FirstRelease:          false,
			ReleaseVersion:        semver.MustParse("2.1.0"),
			ReleaseTag:            "v2.1.0",
			ChangeLevel:           changeLevelMinor,
			CreatedTag:            true,
			CreatedRelease:        true,
			PrereleaseHookOutput:  "hello to my friends reading stdout\n",
			PreTagHookOutput:      "hello to my friends reading stdout\n",
		}, got)
		taggedSha, err := runCmd(ctx, &runCmdOpts{
			dir: repos.origin,
		}, "git", "rev-parse", "v2.1.0")
		require.NoError(t, err)
		require.Equal(t, repos.taggedCommits["head"], taggedSha)
	})

	t.Run("first release", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, "git", "checkout", "third")
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CreateRelease(gomock.Any(), "orgName", "repoName", "x1.0.0", "", false).Return(
			&github.RepoRelease{
				ID:        1,
				UploadURL: "localhost",
			}, nil,
		)
		githubClient.EXPECT().PublishRelease(gomock.Any(), "orgName", "repoName", "", int64(1)).Return(nil)
		runner := Runner{
			CheckoutDir:   repos.clone,
			Ref:           repos.taggedCommits["third"],
			TagPrefix:     "x",
			Repo:          "orgName/repoName",
			PushRemote:    "origin",
			GithubClient:  githubClient,
			CreateRelease: true,
			InitialTag:    "x1.0.0",
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			FirstRelease:   true,
			ReleaseTag:     "x1.0.0",
			ReleaseVersion: semver.MustParse("1.0.0"),
			ChangeLevel:    changeLevelNone,
			CreatedTag:     true,
			CreatedRelease: true,
		}, got)
	})

	t.Run("tags $RELEASE_TARGET", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)

		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 2,
				Commits: []string{repos.taggedCommits["fourth"], repos.taggedCommits["head"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 2}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["fourth"]).Return(
			[]github.BasePull{{Number: 1, MergeCommitSha: mergeSha, Labels: []string{labelMinor}}}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["head"]).Return(
			[]github.BasePull{}, nil,
		)
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
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["head"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			PushRemote:   "origin",
			GithubClient: githubClient,
			CreateTag:    true,
			PreTagHook:   preHook,
			GithubToken:  "token",
			TempDir:      t.TempDir(),
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			FirstRelease:          false,
			ReleaseTag:            "v2.1.0",
			ReleaseVersion:        semver.MustParse("2.1.0"),
			PreviousVersion:       "2.0.0",
			PreviousRef:           "v2.0.0",
			PreviousStableRef:     "v2.0.0",
			PreviousStableVersion: "2.0.0",
			ChangeLevel:           changeLevelMinor,
			CreatedTag:            true,
			CreatedRelease:        false,
		}, got)
		target := mustRunCmd(t, repos.origin, "git", "rev-parse", "v2.1.0")
		// We don't know what the commit sha will be, but it should be different from head.
		require.NotEqual(t, repos.taggedCommits["head"], target)
	})

	t.Run("prerelease hook exits 10 to skip release", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)

		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 2,
				Commits: []string{repos.taggedCommits["fourth"], repos.taggedCommits["head"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 2}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["fourth"]).Return(
			[]github.BasePull{{Number: 1, MergeCommitSha: mergeSha, Labels: []string{labelMinor}}}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["head"]).Return(
			[]github.BasePull{}, nil,
		)
		preHook := `echo aborting; exit 10`
		runner := Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["head"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			PushRemote:   "origin",
			GithubClient: githubClient,
			CreateTag:    true,
			PreTagHook:   preHook,
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			FirstRelease:          false,
			ReleaseTag:            "v2.1.0",
			ReleaseVersion:        semver.MustParse("2.1.0"),
			PreviousVersion:       "2.0.0",
			PreviousRef:           "v2.0.0",
			PreviousStableRef:     "v2.0.0",
			PreviousStableVersion: "2.0.0",
			ChangeLevel:           changeLevelMinor,
			CreatedTag:            false,
			CreatedRelease:        false,
			PrereleaseHookOutput:  "aborting\n",
			PrereleaseHookAborted: true,
			PreTagHookOutput:      "aborting\n",
			PreTagHookAborted:     true,
		}, got)
	})

	t.Run("pre-release hook failure without shouldCreateTag", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)

		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 2,
				Commits: []string{repos.taggedCommits["fourth"], repos.taggedCommits["head"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 2}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["fourth"]).Return(
			[]github.BasePull{{Number: 1, MergeCommitSha: mergeSha, Labels: []string{labelMinor}}}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["head"]).Return(
			[]github.BasePull{}, nil,
		)
		preHook := `
echo failure
echo this is an error >&2
echo this is another error >&2
exit 1
`
		var stderr bytes.Buffer
		var stdout bytes.Buffer
		runner := Runner{
			Stdout:       &stdout,
			Stderr:       &stderr,
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["head"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			PushRemote:   "origin",
			GithubClient: githubClient,
			PreTagHook:   preHook,
		}
		_, err := runner.Run(ctx)
		require.EqualError(t, err, "exit status 1")
		require.Contains(t, stderr.String(), "this is an error\nthis is another error\n")
		require.Contains(t, stdout.String(), "failure\n")
	})

	t.Run("generates release notes from API", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 0,
				Commits: []string{repos.taggedCommits["v2.0.0"], repos.taggedCommits["head"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 0}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["v2.0.0"]).Return(
			[]github.BasePull{{Number: 1, MergeCommitSha: mergeSha, Labels: []string{labelMinor}}}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["head"]).Return(
			[]github.BasePull{{Number: 1, MergeCommitSha: mergeSha, Labels: []string{labelMinor}}}, nil,
		)
		githubClient.EXPECT().GenerateReleaseNotes(gomock.Any(), "orgName", "repoName", "v2.1.0", "v2.0.0").Return(
			"release notes", nil,
		)
		githubClient.EXPECT().CreateRelease(gomock.Any(), "orgName", "repoName", "v2.1.0", "release notes", false).Return(
			&github.RepoRelease{
				ID:        1,
				UploadURL: "localhost",
			}, nil,
		)
		githubClient.EXPECT().PublishRelease(gomock.Any(), "orgName", "repoName", "", int64(1)).Return(nil)

		runner := Runner{
			CheckoutDir:   repos.clone,
			Ref:           repos.taggedCommits["head"],
			TagPrefix:     "v",
			Repo:          "orgName/repoName",
			PushRemote:    "origin",
			GithubClient:  githubClient,
			CreateRelease: true,
		}
		got, err := runner.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:           "v2.0.0",
			PreviousVersion:       "2.0.0",
			PreviousStableRef:     "v2.0.0",
			PreviousStableVersion: "2.0.0",
			FirstRelease:          false,
			ReleaseTag:            "v2.1.0",
			ReleaseVersion:        semver.MustParse("2.1.0"),
			ChangeLevel:           changeLevelMinor,
			CreatedTag:            true,
			CreatedRelease:        true,
		}, got)
	})

	t.Run("shallow clone", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, "git", "pull", "--depth=1")
		runner := &Runner{
			CheckoutDir: repos.clone,
		}
		_, err := runner.Run(ctx)
		require.EqualError(t, err, "shallow clones are not supported")
	})

	t.Run("not a git repo", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, "rm", "-rf", ".git")
		runner := &Runner{
			CheckoutDir: repos.clone,
		}
		_, err := runner.Run(ctx)
		require.ErrorContains(t, err, "not a git repository")
	})

	t.Run("api error", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			nil, errors.New("api error"),
		)
		_, err := (&Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["head"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			GithubClient: githubClient,
		}).Run(ctx)
		require.EqualError(t, err, "api error")
	})

	t.Run("release error deletes tag", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 1,
				Commits: []string{repos.taggedCommits["head"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 0}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["head"]).Return(
			[]github.BasePull{{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelBreaking}}}, nil,
		)
		githubClient.EXPECT().GenerateReleaseNotes(gomock.Any(), "orgName", "repoName", "v3.0.0", "v2.0.0").Return(
			"release notes", nil,
		)
		githubClient.EXPECT().CreateRelease(gomock.Any(), "orgName", "repoName", "v3.0.0", "release notes", false).Return(
			nil, errors.New("release error"),
		)
		runner := Runner{
			CheckoutDir:   repos.clone,
			Ref:           repos.taggedCommits["head"],
			TagPrefix:     "v",
			Repo:          "orgName/repoName",
			PushRemote:    "origin",
			GithubClient:  githubClient,
			CreateRelease: true,
		}
		_, err := runner.Run(ctx)
		require.EqualError(t, err, "release error")
		ok, err := localTagExists(ctx, repos.origin, "v3.0.0")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("upload error deletes release", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 1,
				Commits: []string{repos.taggedCommits["head"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 0}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["head"]).Return(
			[]github.BasePull{{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelBreaking}}}, nil,
		)
		githubClient.EXPECT().GenerateReleaseNotes(gomock.Any(), "orgName", "repoName", "v3.0.0", "v2.0.0").Return(
			"release notes", nil,
		)
		githubClient.EXPECT().CreateRelease(gomock.Any(), "orgName", "repoName", "v3.0.0", "release notes", false).Return(
			&github.RepoRelease{
				ID:        1,
				UploadURL: "localhost",
			}, nil,
		)
		githubClient.EXPECT().UploadAsset(gomock.Any(), "localhost", gomock.Any()).Return(errors.New("upload error"))
		githubClient.EXPECT().DeleteRelease(gomock.Any(), "orgName", "repoName", int64(1)).Return(nil)
		preHook := `
#!/bin/sh
set -e

echo foo > "$ASSETS_DIR/foo.txt"
echo bar > "$ASSETS_DIR/bar.txt"
`
		runner := &Runner{
			CheckoutDir:   repos.clone,
			Ref:           repos.taggedCommits["head"],
			TagPrefix:     "v",
			Repo:          "orgName/repoName",
			PushRemote:    "origin",
			GithubClient:  githubClient,
			CreateRelease: true,
			PreTagHook:    preHook,
			TempDir:       t.TempDir(),
		}
		_, err := runner.Run(ctx)
		require.ErrorContains(t, err, "upload error")
		ok, err := localTagExists(ctx, repos.origin, "v3.0.0")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("no create tag", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 1,
				Commits: []string{repos.taggedCommits["head"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 0}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["head"]).Return(
			[]github.BasePull{{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelBreaking}}}, nil,
		)
		got, err := (&Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["head"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			PushRemote:   "origin",
			GithubClient: githubClient,
		}).Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:           "v2.0.0",
			PreviousVersion:       "2.0.0",
			PreviousStableRef:     "v2.0.0",
			PreviousStableVersion: "2.0.0",
			FirstRelease:          false,
			ReleaseVersion:        semver.MustParse("3.0.0"),
			ReleaseTag:            "v3.0.0",
			ChangeLevel:           changeLevelMajor,
		}, got)
	})

	t.Run("non-matching ref prevents tag", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.0.0", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 1,
				Commits: []string{"fake"},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 0}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", "fake").Return(
			[]github.BasePull{{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelBreaking}}}, nil,
		)
		got, err := (&Runner{
			CheckoutDir:   repos.clone,
			Ref:           repos.taggedCommits["head"],
			TagPrefix:     "v",
			Repo:          "orgName/repoName",
			PushRemote:    "origin",
			GithubClient:  githubClient,
			CreateRelease: true,
			ReleaseRefs:   []string{"fake"},
		}).Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:           "v2.0.0",
			PreviousVersion:       "2.0.0",
			PreviousStableRef:     "v2.0.0",
			PreviousStableVersion: "2.0.0",
			FirstRelease:          false,
			ReleaseVersion:        semver.MustParse("3.0.0"),
			ReleaseTag:            "v3.0.0",
			ChangeLevel:           changeLevelMajor,
		}, got)
	})

	t.Run("V0 prevents bumping to v1", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v0.2.0", repos.taggedCommits["second"], -1).Return(
			&github.CommitComparison{
				AheadBy: 1,
				Commits: []string{repos.taggedCommits["second"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["second"], 0).Return(
			&github.CommitComparison{AheadBy: 0}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["second"]).Return(
			[]github.BasePull{{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelBreaking}}}, nil,
		)
		got, err := (&Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["second"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			PushRemote:   "origin",
			GithubClient: githubClient,
			V0:           true,
		}).Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:           "v0.2.0",
			PreviousVersion:       "0.2.0",
			PreviousStableRef:     "v0.2.0",
			PreviousStableVersion: "0.2.0",
			FirstRelease:          false,
			ReleaseVersion:        semver.MustParse("0.3.0"),
			ReleaseTag:            "v0.3.0",
			ChangeLevel:           changeLevelMinor,
		}, got)
	})

	t.Run("V0 errors when previous version is v1", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		_, err := (&Runner{
			CheckoutDir: repos.clone,
			Ref:         repos.taggedCommits["third"],
			TagPrefix:   "v",
			Repo:        "orgName/repoName",
			V0:          true,
		}).Run(ctx)
		require.EqualError(t, err, `v0 flag is set, but previous version "1.0.0" has major version > 0`)
	})

	t.Run("iterates prerelease", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		repos := setupGit(t)
		mustRunCmd(t, repos.clone, "git", "tag", "v2.1.0-rc.1", "fifth")
		githubClient := mocks.NewMockGithubClient(gomock.NewController(t))
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", "v2.1.0-rc.1", repos.taggedCommits["head"], -1).Return(
			&github.CommitComparison{
				AheadBy: 1,
				Commits: []string{repos.taggedCommits["head"]},
			}, nil,
		)
		githubClient.EXPECT().CompareCommits(gomock.Any(), "orgName", "repoName", mergeSha, repos.taggedCommits["head"], 0).Return(
			&github.CommitComparison{AheadBy: 0}, nil,
		)
		githubClient.EXPECT().ListMergedPullsForCommit(gomock.Any(), "orgName", "repoName", repos.taggedCommits["head"]).Return(
			[]github.BasePull{{Number: 2, MergeCommitSha: mergeSha, Labels: []string{labelMinor, labelPrerelease}}}, nil,
		)
		got, err := (&Runner{
			CheckoutDir:  repos.clone,
			Ref:          repos.taggedCommits["head"],
			TagPrefix:    "v",
			Repo:         "orgName/repoName",
			PushRemote:   "origin",
			GithubClient: githubClient,
		}).Run(ctx)
		require.NoError(t, err)
		require.Equal(t, &Result{
			PreviousRef:           "v2.1.0-rc.1",
			PreviousVersion:       "2.1.0-rc.1",
			PreviousStableRef:     "v2.0.0",
			PreviousStableVersion: "2.0.0",
			FirstRelease:          false,
			ReleaseVersion:        semver.MustParse("2.1.0-rc.2"),
			ReleaseTag:            "v2.1.0-rc.2",
			ChangeLevel:           changeLevelMinor,
		}, got)
	})
}
