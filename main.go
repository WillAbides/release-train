package main

import (
	"context"
	"os"

	releasetrain "github.com/willabides/release-train/v3/internal/cmd/release-train"
)

var version = "dev"

func main() {
	ctx := context.Background()
	releasetrain.Run(ctx, version, os.Args[1:])
}
