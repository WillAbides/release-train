package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_getPrevVersion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	_, e := runCmd(ctx, &runCmdOpts{
		dir: dir,
		env: map[string]string{
			"GIT_AUTHOR_NAME":    "foo",
			"GIT_COMMITTER_NAME": "foo",
			"EMAIL":              "foo@example.com",
		},
		stdout: os.Stdout,
		stderr: os.Stderr,
	}, "sh", "-c", `
git init
git commit --allow-empty -m "first"
git tag v0.1.0
git tag foo0.1.0
git tag v0.1.1
git tag foo0.1.1
git tag v0.2.0
git tag foo0.2.0
git tag v1.0.0
git commit --allow-empty -m "second"
git commit --allow-empty -m "third"
git tag v2.0.0
git tag foo
git commit --allow-empty -m "forth"
git tag bar
`)
	require.NoError(t, e)

	t.Run("", func(t *testing.T) {
		opts := getPrevTagOpts{
			RepoDir:  dir,
			Prefixes: []string{"v"},
		}
		got, err := getPrevTag(ctx, &opts)
		require.NoError(t, err)
		require.Equal(t, "v2.0.0", got)
	})

	t.Run("", func(t *testing.T) {
		opts := getPrevTagOpts{
			RepoDir:  dir,
			Prefixes: []string{"v", "bar"},
		}
		got, err := getPrevTag(ctx, &opts)
		require.NoError(t, err)
		require.Equal(t, "v2.0.0", got)
	})
}
