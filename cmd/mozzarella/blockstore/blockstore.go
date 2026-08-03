//go:build !lite

package blockstore

import (
	"github.com/davleng/mozzarella/cmd/mozzarella/blockstore/compact"
	"github.com/davleng/mozzarella/cmd/mozzarella/blockstore/dumpbatches"
	"github.com/davleng/mozzarella/cmd/mozzarella/blockstore/dumpshreds"
	"github.com/davleng/mozzarella/cmd/mozzarella/blockstore/statdatarate"
	"github.com/davleng/mozzarella/cmd/mozzarella/blockstore/statentries"
	"github.com/davleng/mozzarella/cmd/mozzarella/blockstore/tarblocks"
	"github.com/davleng/mozzarella/cmd/mozzarella/blockstore/verifydata"
	"github.com/davleng/mozzarella/cmd/mozzarella/blockstore/yaml"
	"github.com/spf13/cobra"
)

var Cmd = cobra.Command{
	Use:   "blockstore",
	Short: "Access blockstore database",
}

func init() {
	Cmd.AddCommand(
		&compact.Cmd,
		&dumpshreds.Cmd,
		&dumpbatches.Cmd,
		&statdatarate.Cmd,
		&statentries.Cmd,
		&tarblocks.Cmd,
		&verifydata.Cmd,
		&yaml.Cmd,
	)
}
