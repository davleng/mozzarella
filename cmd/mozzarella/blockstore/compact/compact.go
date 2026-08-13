//go:build !lite

package compact

import (
	"context"

	"github.com/davleng/mozzarella/blockstore"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var Cmd = cobra.Command{
	Use:   "compact <database>",
	Short: "Compact blockstore database",
	Args:  cobra.ExactArgs(1),
}

func init() {
	Cmd.Run = run
}

func run(_ *cobra.Command, args []string) {
	db, err := blockstore.OpenReadWrite(args[0])
	if err != nil {
		klog.Exitf("Failed to open blockstore: %s", err)
	}
	defer db.Close()
	klog.Infof("Compacting database")
	if err := db.Compact(context.Background()); err != nil {
		klog.Exitf("Failed to compact blockstore: %s", err)
	}

	klog.Infof("Done")
}
