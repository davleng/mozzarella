//go:build !lite

package dumpshreds

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/davleng/mozzarella/blockstore"
	"github.com/davleng/mozzarella/cmd/mozzarella/blockstore/util"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var Cmd = cobra.Command{
	Use:   "dump-shreds <database> <out> <slots>",
	Short: "Dump shreds to file system",
	Long: `dump-shreds writes raw code and data shreds from the blockstore database to the file system.
Slots can be specified as integers or ranges separated by comma.

Creates file paths ./<out>/<slot>/<type><index>
  type is either 'd' (data) or 'c' (code)

File paths are written to stdout.`,
	Example: `    dump-shreds ./database ./shreds 1,2,100:200
      ./shreds/1/d000
      ./shreds/1/d001
      ...
      ./shreds/2/d000
      ./shreds/2/d001
      ...
      ./shreds/100/d000
      ./shreds/100/d001
      ...
      ./shreds/101/d000
      ./shreds/101/d001`,
	Args: cobra.ExactArgs(3),
}

func init() {
	Cmd.Run = run
}

func run(_ *cobra.Command, args []string) {
	dbPath := args[0]
	outPath := args[1]

	db, err := blockstore.OpenReadOnly(dbPath)
	if err != nil {
		klog.Exitf("Failed to open blockstore: %s", err)
	}
	defer db.Close()

	if err := os.Mkdir(outPath, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
		klog.Exit(err)
	}

	slots, ok := util.ParseInts(args[2])
	if !ok {
		klog.Exit("Invalid slots parameter")
	}
	slots.Iter(func(slot uint64) bool {
		err := dumpSlot(db, outPath, slot)
		if err != nil {
			klog.Warningf("Failed to dump slot %d: %s", slot, err)
		}
		return true
	})
}

func dumpSlot(db *blockstore.DB, outPath string, slot uint64) error {
	slotPath := filepath.Join(outPath, strconv.FormatUint(slot, 10))
	if err := os.Mkdir(slotPath, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}

	if err := dumpShreds(db, slotPath, slot, blockstore.CfCodeShred, "c"); err != nil {
		return err
	}
	if err := dumpShreds(db, slotPath, slot, blockstore.CfDataShred, "d"); err != nil {
		return err
	}
	return nil
}

func dumpShreds(
	db *blockstore.DB, slotPath string, slot uint64,
	columnFamily string,
	namePrefix string,
) error {
	iter, err := db.NewIterator(columnFamily)
	if err != nil {
		return err
	}
	defer iter.Close()
	prefix := blockstore.MakeShredKey(slot, 0)
	iter.Seek(prefix[:])
	for iter.Valid() {
		curSlot, curIndex, ok := blockstore.ParseShredKey(iter.Key())
		if !ok || curSlot != slot {
			break
		}
		p := filepath.Join(slotPath, fmt.Sprintf("%s%04d", namePrefix, curIndex))
		if err := os.WriteFile(p, iter.Value(), 0644); err != nil {
			return err
		}
		fmt.Println(p)
		iter.Next()
	}
	return nil
}
