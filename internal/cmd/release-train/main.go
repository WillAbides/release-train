package releasetrain

import (
	"context"
	_ "embed"

	"github.com/alecthomas/kong"
	"gopkg.in/yaml.v3"
)

//go:embed vars.yaml
var varsYaml []byte

type contextKey string

const (
	versionKey contextKey = "version"
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

func Run(ctx context.Context, version string, args []string) {
	ctx = withVersion(ctx, version)

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
		kong.Description("release-train keeps a-rollin' down to San Antone"),
		vars,
	)
	if err != nil {
		panic(err)
	}
	k, err := parser.Parse(args)
	parser.FatalIfErrorf(err)
	k.FatalIfErrorf(k.Run(&root))
}
