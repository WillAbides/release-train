package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	logger := slog.Default()
	if opts.noLog {
		logger = slog.New(slog.DiscardHandler)
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

// runCmdHandleLines lets us run a command that has a large output and handle
// one line at a time without waiting for the command to finish or buffering
// the entire output in memory.
func runCmdHandleLines(
	ctx context.Context,
	dir string,
	cmdLine []string,
	handleLine func(line string, cancel context.CancelFunc),
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdLine[0], cmdLine[1:]...)
	cmd.Dir = dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	err = cmd.Start()
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		handleLine(line, cancel)
	}
	err = cmd.Wait()
	if err == nil {
		return nil
	}
	if !errors.Is(err, context.Canceled) && err.Error() != "signal: killed" {
		return err
	}
	return nil
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
	const partCount = 2
	if len(parts) < partCount {
		return "", fmt.Errorf("remote url is not a properly formated github repo url: %s", orig)
	}
	return strings.Join(parts[len(parts)-partCount:], "/"), nil
}

func addCmdEnv(cmd *exec.Cmd, key string, val any) {
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", key, val))
}
