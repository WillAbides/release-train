package releasetrain

import (
	"context"
	"fmt"
	"os"

	"github.com/Masterminds/semver/v3"
	"github.com/willabides/release-train-action/v2/internal/prev"
)

type prevCmd struct {
	Ref        string `kong:"default=HEAD,help=${ref_help}"`
	TagPrefix  string `kong:"default=v,help=${tag_prefix_help}"`
	Constraint string `kong:"help=${constraint_help}"`
	InitialTag string `kong:"help=${initial_tag_help},default=${initial_tag_default}"`
}

func (c *prevCmd) Run(ctx context.Context, root *rootCmd) error {
	opts := prev.Options{
		Head:     c.Ref,
		RepoDir:  root.CheckoutDir,
		Fallback: c.InitialTag,
	}
	if c.TagPrefix != "" {
		opts.Prefixes = []string{c.TagPrefix}
	}
	if c.Constraint != "" {
		var err error
		opts.Constraints, err = semver.NewConstraint(c.Constraint)
		if err != nil {
			return err
		}
	}
	ver, err := prev.GetPrevTag(ctx, &opts)
	if err != nil {
		return err
	}
	if ver == "" {
		return fmt.Errorf("no previous tag found")
	}
	_, err = fmt.Fprintln(os.Stdout, ver)
	return err
}
