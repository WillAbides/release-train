package testutil

import (
	"testing"

	"github.com/golang/mock/gomock"
	mock "github.com/willabides/release-train/v3/internal/mock"
)

func MockGithubClient(t *testing.T) *mock.MockGithubClient {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	return mock.NewMockGithubClient(ctrl)
}
