package main

import (
	"testing"

	"go.uber.org/mock/gomock"
)

func mockGithubClient(t *testing.T) *MockGithubClient {
	return NewMockGithubClient(gomock.NewController(t))
}
