package releasetrain

import (
	"context"
	_ "embed"
	"os"

	"github.com/alecthomas/kong"
	"golang.org/x/exp/slog"
	"gopkg.in/yaml.v3"
)

//go:embed vars.yaml
var varsYaml []byte

type contextKey string

const (
	versionKey contextKey = "version"
	loggerKey  contextKey = "logger"
)

func withVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, versionKey, version)
}

func getVersion(ctx context.Context) string {
	if v, ok := ctx.Value(versionKey).(string); ok {
		return v
	}
	return ""
}

func withLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

func logger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

func Run(ctx context.Context, version string, args []string) {
	ctx = withVersion(ctx, version)
	ctx = withLogger(ctx, slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	))

	vars := kong.Vars{}
	err := yaml.Unmarshal(varsYaml, &vars)
	if err != nil {
		panic(err)
	}
	vars["version"] = version

	var root rootCmd
	parser, err := kong.New(
		&root,
		kong.BindTo(ctx, (*context.Context)(nil)),
		vars,
	)
	if err != nil {
		panic(err)
	}
	k, err := parser.Parse(args)
	parser.FatalIfErrorf(err)
	k.FatalIfErrorf(k.Run(&root))
}
