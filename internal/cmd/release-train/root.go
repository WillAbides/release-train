package releasetrain

import (
	"github.com/alecthomas/kong"
)

type rootCmd struct {
	CheckoutDir string     `kong:"short=C,default='.',help=${checkout_dir_help}"`
	Release     releaseCmd `kong:"cmd,help='Release a new version.'"`
	Prev        prevCmd    `kong:"cmd,help='Get the previous version.'"`
	Action      actionCmd  `kong:"cmd"`
	Version     kong.VersionFlag
}
