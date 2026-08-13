//go:build !lite

package blockstore

import (
	"fmt"
	"sort"

	"github.com/davleng/mozzarella/shred"
	"k8s.io/klog/v2"
)

// BlockWalkI abstracts iterators over block data.
//
// The main (and only) implementation in this package is BlockWalk.
type BlockWalkI interface {
	Seek(slot uint64) (ok bool)
	SlotsAvailable() (total uint64)
	Next() (meta *SlotMeta, ok bool)
	Close()

	// Entries returns the block contents of a slot.
	//
	// The outer returned slice contains batches of entries.
	// Each batch is made up from multiple shreds and shreds and batches are aligned.
	// The SlotMeta.EntryEndIndexes mark the indexes of the last shreds in each batch,
	// thus `len(SlotMeta.EntryEndIndexes)` equals `len(batches)`.
	//
	// The inner slices are the entries in each shred batch, usually sized one.
	Entries(meta *SlotMeta) (batches [][]shred.Entry, err error)
}

// BlockWalk walks blocks in ascending order over multiple Pebble databases.
type BlockWalk struct {
	handles       []WalkHandle // sorted
	shredRevision int

	root *Iterator
}

func NewBlockWalk(handles []WalkHandle, shredRevision int) (*BlockWalk, error) {
	if err := sortWalkHandles(handles, shredRevision); err != nil {
		return nil, err
	}
	return &BlockWalk{handles: handles, shredRevision: shredRevision}, nil
}

func (m *BlockWalk) Seek(slot uint64) bool {
	for len(m.handles) > 0 {
		h := m.handles[0]
		if slot < h.Start {
			return false
		}
		if slot <= h.Stop {
			h.Start = slot
			return true
		}
		m.pop()
	}
	return false
}

func (m *BlockWalk) SlotsAvailable() (total uint64) {
	if len(m.handles) == 0 {
		return 0
	}
	start := m.handles[0].Start
	for _, h := range m.handles {
		if h.Start > start {
			return
		}
		stop := h.Stop + 1
		total += stop - start
		start = stop
	}
	return
}

func (m *BlockWalk) Next() (meta *SlotMeta, ok bool) {
	if len(m.handles) == 0 {
		return nil, false
	}
	h := m.handles[0]
	if m.root == nil {
		var err error
		m.root, err = h.DB.NewIterator(CfRoot)
		if err != nil {
			return nil, false
		}
		key := MakeSlotKey(h.Start)
		m.root.Seek(key[:])
	}
	if !m.root.Valid() {
		m.pop()
		return m.Next()
	}
	slot, ok := ParseSlotKey(m.root.Key())
	if !ok {
		klog.Exitf("Invalid slot key: %x", m.root.Key())
	}
	if slot > h.Stop {
		m.pop()
		return m.Next()
	}
	h.Start = slot
	meta, err := h.DB.GetSlotMeta(slot)
	if err != nil {
		klog.Errorf("FATAL: invalid slot meta at slot %d, aborting CAR generation: %s", slot, err)
		return nil, false
	}
	m.root.Next()
	return meta, true
}

func (m *BlockWalk) Current() *DB {
	if len(m.handles) == 0 {
		return nil
	}
	return m.handles[0].DB
}

func (m *BlockWalk) Entries(meta *SlotMeta) ([][]shred.Entry, error) {
	h := m.handles[0]
	mapping, err := h.DB.GetEntries(meta, m.shredRevision)
	if err != nil {
		return nil, err
	}
	batches := make([][]shred.Entry, len(mapping))
	for i, batch := range mapping {
		batches[i] = batch.Entries
	}
	return batches, nil
}

func (m *BlockWalk) pop() {
	if m.root != nil {
		_ = m.root.Close()
		m.root = nil
	}
	_ = m.handles[0].DB.Close()
	m.handles = m.handles[1:]
}

func (m *BlockWalk) Close() {
	if m.root != nil {
		_ = m.root.Close()
		m.root = nil
	}
	for _, h := range m.handles {
		_ = h.DB.Close()
	}
	m.handles = nil
}

type WalkHandle struct {
	DB    *DB
	Start uint64
	Stop  uint64
}

func sortWalkHandles(h []WalkHandle, shredRevision int) error {
	for i, db := range h {
		start, err := getLowestCompletedSlot(db.DB, shredRevision)
		if err != nil {
			return err
		}
		stop, err := db.DB.MaxRoot()
		if err != nil {
			return err
		}
		h[i] = WalkHandle{Start: start, Stop: stop, DB: db.DB}
	}
	sort.Slice(h, func(i, j int) bool { return h[i].Start < h[j].Start })
	return nil
}

func getLowestCompletedSlot(d *DB, shredRevision int) (uint64, error) {
	iter, err := d.NewIterator(CfMeta)
	if err != nil {
		return 0, err
	}
	defer iter.Close()
	iter.SeekToFirst()
	const maxTries = 32
	for i := 0; iter.Valid() && i < maxTries; i++ {
		slot, ok := ParseSlotKey(iter.Key())
		if !ok {
			return 0, fmt.Errorf("getLowestCompletedSlot: invalid slot key: %x", iter.Key())
		}
		meta, err := ParseBincode[SlotMeta](iter.Value())
		if err != nil {
			return 0, fmt.Errorf("getLowestCompletedSlot: invalid meta for slot %d", slot)
		}
		if _, err = d.GetEntries(meta, shredRevision); err == nil {
			return slot, nil
		}
		iter.Next()
	}
	return 0, fmt.Errorf("failed to find a valid complete slot in DB")
}
