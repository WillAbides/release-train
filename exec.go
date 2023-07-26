package main

import (
	"errors"
	"fmt"
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

func runCmd(dir string, env map[string]string, command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	out, err := cmd.Output()
	if err != nil {
		exitErr := asExitErr(err)
		if exitErr != nil {
			err = errors.Join(err, errors.New(string(exitErr.Stderr)))
		}
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

func getGithubRepoFromRemote(dir, remote string) (string, error) {
	orig, err := runCmd(dir, nil, "git", "remote", "get-url", remote)
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
