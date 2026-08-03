package tpu_udp

import (
	"github.com/davleng/mozzarella/cmd/mozzarella/tpu_udp/pcap"
	"github.com/davleng/mozzarella/cmd/mozzarella/tpu_udp/proxy"
	"github.com/davleng/mozzarella/cmd/mozzarella/tpu_udp/sniff"
	"github.com/spf13/cobra"
)

var Cmd = cobra.Command{
	Use:   "tpu-udp",
	Short: "TPU/UDP tools",
}

func init() {
	Cmd.AddCommand(
		&pcap.Cmd,
		&proxy.Cmd,
		&sniff.Cmd,
	)
}
