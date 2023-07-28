package main

import (
	"testing"

	"github.com/golang/mock/gomock"
)

func mockGithubClient(t *testing.T) *MockGithubClient {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	return NewMockGithubClient(ctrl)
}
