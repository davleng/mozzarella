//go:build !lite

// Package blockstore is a read-only client for the Solana blockstore database.
//
// For the reference implementation in Rust, see here:
// https://docs.rs/solana-ledger/latest/solana_ledger/blockstore/struct.Blockstore.html
//
// # Compatibility
//
// We aim to support all Solana Rust versions since mainnet genesis.
package blockstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/cockroachdb/pebble/v2"
)

// Column families
const (
	// CfDefault is the default logical column family.
	CfDefault = "default"

	// CfMeta contains slot metadata (SlotMeta)
	//
	// Similar to a block header, but not cryptographically authenticated.
	CfMeta = "meta"

	// CfErasureMeta contains erasure coding metadata
	CfErasureMeta = "erasure_meta"

	// CfRoot is a single cell specifying the current root slot number
	CfRoot = "root"

	// CfDataShred contains ledger data.
	//
	// One or more shreds make up a single entry.
	// The shred => entry surjection is indicated by SlotMeta.EntryEndIndexes
	CfDataShred = "data_shred"

	// CfCodeShred contains FEC shreds used to fix data shreds
	CfCodeShred = "code_shred"

	// CfDeadSlots contains slots that have been marked as dead
	CfDeadSlots = "dead_slots"

	CfBlockHeight = "block_height"

	CfBankHash = "bank_hashes"

	CfTxStatus = "transaction_status"

	CfTxStatusIndex = "transaction_status_index"

	CfAddressSig = "address_signatures"

	CfTxMemos = "transaction_memos"

	CfRewards = "rewards"

	CfBlockTime = "blocktime"

	CfPerfSamples = "perf_samples"

	CfProgramCosts = "program_costs"

	CfOptimisticSlots = "optimistic_slots"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrDeadSlot         = errors.New("dead slot")
	ErrInvalidShredData = errors.New("invalid shred data")
)

// DB is the Pebble-backed blockstore wrapper.
type DB struct {
	DB *pebble.DB
}

// Iterator is a logical column-family iterator. Keys returned by Iterator are
// business keys; the internal column-family prefix is hidden from callers.
type Iterator struct {
	iter   *pebble.Iterator
	prefix []byte
}

func OpenReadWrite(path string) (*DB, error) {
	return open(path, false)
}

// OpenReadOnly opens a point-in-time read-only Pebble database.
func OpenReadOnly(path string) (*DB, error) {
	return open(path, true)
}

// OpenSecondary is not supported by Pebble. Use OpenReadOnly for a read-only
// view of a Pebble database.
func OpenSecondary(_ string, _ string) (*DB, error) {
	return nil, errors.New("blockstore secondary mode is not supported by Pebble")
}

func open(path string, readOnly bool) (*DB, error) {
	opts := &pebble.Options{ReadOnly: readOnly}
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, err
	}
	return &DB{DB: db}, nil
}

func (d *DB) Get(columnFamily string, key []byte) ([]byte, error) {
	physicalKey, err := makePhysicalKey(columnFamily, key)
	if err != nil {
		return nil, err
	}
	value, closer, err := d.DB.Get(physicalKey)
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer closer.Close()
	return append([]byte(nil), value...), nil
}

// Set writes a value under a logical column family.
func (d *DB) Set(columnFamily string, key, value []byte) error {
	physicalKey, err := makePhysicalKey(columnFamily, key)
	if err != nil {
		return err
	}
	return d.DB.Set(physicalKey, value, pebble.Sync)
}

func (d *DB) NewIterator(columnFamily string) (*Iterator, error) {
	prefix, err := columnFamilyPrefix(columnFamily)
	if err != nil {
		return nil, err
	}
	iter, err := d.DB.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: prefixUpperBound(prefix),
	})
	if err != nil {
		return nil, err
	}
	return &Iterator{iter: iter, prefix: prefix}, nil
}

func (i *Iterator) First() bool { return i.iter.First() }

func (i *Iterator) Seek(key []byte) bool {
	physicalKey := make([]byte, 0, len(i.prefix)+len(key))
	physicalKey = append(physicalKey, i.prefix...)
	physicalKey = append(physicalKey, key...)
	return i.iter.SeekGE(physicalKey)
}

func (i *Iterator) SeekToFirst() bool { return i.First() }

func (i *Iterator) SeekToLast() bool { return i.iter.Last() }

func (i *Iterator) Next() bool { return i.iter.Next() }

func (i *Iterator) Valid() bool {
	return i.iter.Valid() && len(i.iter.Key()) >= len(i.prefix)
}

func (i *Iterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return append([]byte(nil), i.iter.Key()[len(i.prefix):]...)
}

func (i *Iterator) Value() []byte {
	if !i.Valid() {
		return nil
	}
	value, err := i.iter.ValueAndErr()
	if err != nil {
		return nil
	}
	return append([]byte(nil), value...)
}

func (i *Iterator) Error() error { return i.iter.Error() }

func (i *Iterator) Close() error { return i.iter.Close() }

func (d *DB) Compact(ctx context.Context) error {
	return d.DB.Compact(ctx, nil, nil, false)
}

func (d *DB) Close() error {
	return d.DB.Close()
}

func columnFamilyPrefix(name string) ([]byte, error) {
	prefix, ok := map[string]byte{
		CfDefault: 1, CfMeta: 2, CfRoot: 3, CfDataShred: 4, CfCodeShred: 5,
		CfErasureMeta: 6, CfDeadSlots: 7, CfBlockHeight: 8, CfBankHash: 9,
		CfTxStatus: 10, CfTxStatusIndex: 11, CfAddressSig: 12, CfTxMemos: 13,
		CfRewards: 14, CfBlockTime: 15, CfPerfSamples: 16, CfProgramCosts: 17,
		CfOptimisticSlots: 18,
	}[name]
	if !ok {
		return nil, fmt.Errorf("unknown column family %q", name)
	}
	return []byte{prefix}, nil
}

func makePhysicalKey(columnFamily string, key []byte) ([]byte, error) {
	prefix, err := columnFamilyPrefix(columnFamily)
	if err != nil {
		return nil, err
	}
	return append(prefix, key...), nil
}

func prefixUpperBound(prefix []byte) []byte {
	upper := append([]byte(nil), prefix...)
	for i := len(upper) - 1; i >= 0; i-- {
		if upper[i] != 0xff {
			upper[i]++
			return upper[:i+1]
		}
	}
	return nil
}
