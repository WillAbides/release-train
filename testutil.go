package main

import (
	"testing"

	"github.com/willabides/release-train/v3/internal/mocks"
	"go.uber.org/mock/gomock"
)

func mockGithubClient(t *testing.T) *mocks.MockGithubClient {
	return mocks.NewMockGithubClient(gomock.NewController(t))
}
