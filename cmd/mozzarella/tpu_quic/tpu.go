package tpu_quic

import (
	"github.com/davleng/mozzarella/cmd/mozzarella/tpu_quic/ping"
	"github.com/spf13/cobra"
)

var Cmd = cobra.Command{
	Use:   "tpu-quic",
	Short: "TPU/QUIC tools",
}

func init() {
	Cmd.AddCommand(
		&ping.Cmd,
	)
}
