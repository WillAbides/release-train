package main

import (
	"cmp"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_getPrevTag(t *testing.T) {
	const stdSetup = `
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
	`

	stdSetupEnv := map[string]string{
		"GIT_AUTHOR_NAME":    "foo",
		"GIT_COMMITTER_NAME": "foo",
		"EMAIL":              "foo@example.com",
	}

	for _, test := range []struct {
		name     string
		setupCmd string
		setupEnv map[string]string
		opts     getPrevTagOpts
		wantTag  string
		wantErr  bool
	}{
		{
			name:     "",
			setupCmd: stdSetup,
			opts:     getPrevTagOpts{TagPrefix: "v"},
			wantTag:  "v2.0.0",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			dir := t.TempDir()
			setupOpts := &runCmdOpts{
				dir: dir,
				env: test.setupEnv,
			}
			setupCmd := cmp.Or(test.setupCmd, stdSetup)
			if setupOpts.env == nil {
				setupOpts.env = stdSetupEnv
			}
			_, err := runCmd(ctx, setupOpts, "sh", "-c", setupCmd)
			require.NoError(t, err)

			opts := test.opts
			opts.RepoDir = cmp.Or(opts.RepoDir, dir)
			got, err := getPrevTag(ctx, &opts)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantTag, got)
		})
	}
}
