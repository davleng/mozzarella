//go:build !lite

package blockstore

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/davleng/mozzarella/shred"
	bin "github.com/gagliardetto/binary"
)

// MakeShredKey creates the logical key for CfDataShred or CfCodeShred.
func MakeShredKey(slot, index uint64) (key [16]byte) {
	binary.BigEndian.PutUint64(key[0:8], slot)
	binary.BigEndian.PutUint64(key[8:16], index)
	return
}

// ParseShredKey decodes keys in CfDataShred or CfCodeShred.
func ParseShredKey(key []byte) (slot uint64, index uint64, ok bool) {
	ok = len(key) == 16
	if !ok {
		return
	}
	slot = binary.BigEndian.Uint64(key[0:8])
	index = binary.BigEndian.Uint64(key[8:16])
	return
}

type entryRange struct {
	startIdx, endIdx uint32
}

// entryRanges returns the shred ranges of each entry
func (s *SlotMeta) entryRanges() []entryRange {
	if !s.IsFull() {
		return nil
	}
	indexes := sliceSortedByRange[uint32](s.EntryEndIndexes, 0, uint32(s.Consumed))
	ranges := make([]entryRange, len(indexes))
	begin := uint32(0)
	for i, index := range s.EntryEndIndexes {
		ranges[i] = entryRange{begin, index}
		begin = index + 1
	}
	return ranges
}

type ordered interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 | uintptr |
		float32 | float64 |
		string
}

func sliceSortedByRange[T ordered](list []T, start T, stop T) []T {
	for len(list) > 0 && list[0] < start {
		list = list[1:]
	}
	for len(list) > 0 && list[len(list)-1] >= stop {
		list = list[:len(list)-1]
	}
	return list
}

type Entries struct {
	Entries []shred.Entry
	Raw     []byte
	Shreds  []shred.Shred
}

func (e *Entries) Slot() uint64 {
	if len(e.Shreds) == 0 {
		return math.MaxUint64
	}
	return e.Shreds[0].Slot
}

// DataShredsToEntries reassembles shreds to entries containing transactions.
func DataShredsToEntries(meta *SlotMeta, shreds []shred.Shred) (entries []Entries, err error) {
	ranges := meta.entryRanges()
	for _, r := range ranges {
		parts := shreds[r.startIdx : r.endIdx+1]
		entryBytes := shred.Concat(parts)
		if len(entryBytes) == 0 {
			continue
		}
		var dec bin.Decoder
		dec.SetEncoding(bin.EncodingBin)
		dec.Reset(entryBytes)
		var subEntries SubEntries
		if err := subEntries.UnmarshalWithDecoder(&dec); err != nil {
			return nil, fmt.Errorf("cannot decode entry at %d:[%d-%d]: %w",
				meta.Slot, r.startIdx, r.endIdx, err)
		}
		entries = append(entries, Entries{
			Entries: subEntries.Entries,
			Raw:     entryBytes[:dec.Position()],
			Shreds:  parts,
		})
	}
	return entries, nil
}

type SubEntries struct {
	Entries []shred.Entry
}

func (se *SubEntries) UnmarshalWithDecoder(decoder *bin.Decoder) (err error) {
	// read the number of entries:
	numEntries, err := decoder.ReadUint64(bin.LE)
	if err != nil {
		return fmt.Errorf("failed to read number of entries: %w", err)
	}
	if numEntries > uint64(decoder.Remaining()) {
		return fmt.Errorf("not enough bytes to read %d entries", numEntries)
	}
	// read the entries:
	se.Entries = make([]shred.Entry, numEntries)
	for i := uint64(0); i < numEntries; i++ {
		if err = se.Entries[i].UnmarshalWithDecoder(decoder); err != nil {
			return fmt.Errorf("failed to read entry %d: %w", i, err)
		}
	}
	return
}

func (d *DB) GetEntries(meta *SlotMeta, shredRevision int) ([]Entries, error) {
	shreds, err := d.GetDataShreds(meta.Slot, 0, uint32(meta.Received), shredRevision)
	if err != nil {
		return nil, err
	}
	return DataShredsToEntries(meta, shreds)
}

func (d *DB) GetAllDataShreds(slot uint64, revision int) ([]shred.Shred, error) {
	return d.getAllShreds(CfDataShred, slot, revision)
}

func (d *DB) GetDataShreds(slot uint64, startIdx, endIdx uint32, revision int) ([]shred.Shred, error) {
	iter, err := d.NewIterator(CfDataShred)
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	key := MakeShredKey(slot, uint64(startIdx))
	iter.Seek(key[:])
	return GetDataShredsFromIter(iter, slot, startIdx, endIdx, revision)
}

func GetDataShredsFromIter(iter *Iterator, slot uint64, startIdx, endIdx uint32, revision int) ([]shred.Shred, error) {
	var shreds []shred.Shred
	for i := startIdx; i < endIdx; i++ {
		var curSlot, index uint64
		valid := iter.Valid()
		if valid {
			key := iter.Key()
			if len(key) != 16 {
				continue
			}
			curSlot = binary.BigEndian.Uint64(key)
			index = binary.BigEndian.Uint64(key[8:])
		}
		if !valid || curSlot != slot {
			return nil, fmt.Errorf("missing shreds for slot %d", slot)
		}
		if index != uint64(i) {
			return nil, fmt.Errorf("missing shred %d for slot %d", i, index)
		}
		s := shred.NewShredFromSerialized(iter.Value(), revision)
		if !s.Ok() {
			return nil, fmt.Errorf("failed to deserialize shred %d/%d", slot, i)
		}
		shreds = append(shreds, s)
		iter.Next()
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return shreds, nil
}

func (d *DB) GetDataShred(slot, index uint64, revision int) shred.Shred {
	return d.getShred(CfDataShred, slot, index, revision)
}

func (d *DB) GetRawDataShred(slot, index uint64) ([]byte, error) {
	return d.getRawShred(CfDataShred, slot, index)
}

func (d *DB) GetAllCodeShreds(slot uint64) ([]shred.Shred, error) {
	return d.getAllShreds(CfCodeShred, slot, shred.RevisionV2)
}

func (d *DB) GetCodeShred(slot, index uint64) shred.Shred {
	return d.getShred(CfCodeShred, slot, index, shred.RevisionV2)
}

func (d *DB) GetRawCodeShred(slot, index uint64) ([]byte, error) {
	return d.getRawShred(CfCodeShred, slot, index)
}

func (d *DB) getRawShred(columnFamily string, slot, index uint64) ([]byte, error) {
	key := MakeShredKey(slot, index)
	return d.Get(columnFamily, key[:])
}

func (d *DB) getShred(columnFamily string, slot, index uint64, revision int) (s shred.Shred) {
	value, err := d.getRawShred(columnFamily, slot, index)
	if err != nil {
		return
	}
	return shred.NewShredFromSerialized(value, revision)
}

func (d *DB) getAllShreds(columnFamily string, slot uint64, revision int) ([]shred.Shred, error) {
	iter, err := d.NewIterator(columnFamily)
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	prefix := MakeSlotKey(slot)
	iter.Seek(prefix[:])
	var shreds []shred.Shred
	for iter.Valid() {
		key := iter.Key()
		if len(key) < len(prefix) || string(key[:len(prefix)]) != string(prefix[:]) {
			break
		}
		s := shred.NewShredFromSerialized(iter.Value(), revision)
		if !s.Ok() {
			return nil, fmt.Errorf("invalid shred %d/%d", slot, len(shreds))
		}
		shreds = append(shreds, s)
		iter.Next()
	}
	return shreds, iter.Error()
}
