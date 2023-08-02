package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func asExitErr(err error) *exec.ExitError {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	return nil
}

type runCmdOpts struct {
	env    map[string]string
	dir    string
	stdout io.Writer
	stderr io.Writer
	noLog  bool
}

func runCmd(ctx context.Context, opts *runCmdOpts, command string, args ...string) (string, error) {
	if opts == nil {
		opts = &runCmdOpts{}
	}
	logger := getLogger(ctx)
	if opts.noLog {
		logger = discardLogger
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = opts.dir
	cmd.Env = os.Environ()
	for k, v := range opts.env {
		addCmdEnv(cmd, k, v)
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	if opts.stdout != nil {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, opts.stdout)
	}
	cmd.Stderr = &stderrBuf
	if opts.stderr != nil {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, opts.stderr)
	}
	logger.Debug("running command", "command", command, "args", args)
	err := cmd.Run()
	if err != nil {
		err = errors.Join(err, errors.New(stderrBuf.String()))
		return "", err
	}
	return strings.TrimSpace(stdoutBuf.String()), nil
}

func getGithubRepoFromRemote(ctx context.Context, dir, remote string) (string, error) {
	orig, err := runCmd(ctx, &runCmdOpts{
		dir: dir,
	}, "git", "remote", "get-url", remote)
	if err != nil {
		return "", err
	}
	orig = strings.TrimSpace(orig)
	remoteURL := strings.TrimSuffix(orig, ".git")
	// trim everything before the last : for ssh remotes
	remoteURL = remoteURL[strings.LastIndex(remoteURL, ":")+1:]
	parts := strings.Split(remoteURL, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("remote url is not a properly formated github repo url: %s", orig)
	}
	return strings.Join(parts[len(parts)-2:], "/"), nil
}

func addCmdEnv(cmd *exec.Cmd, key string, val any) {
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", key, val))
}
