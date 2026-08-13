//go:build !lite

package blockstore

import (
	"encoding/hex"
	"fmt"
	bin "github.com/gagliardetto/binary"
)

func ParseBincode[T any](data []byte) (*T, error) {
	dec := bin.NewBinDecoder(data)
	val := new(T)
	err := dec.Decode(val)
	return val, err
}

func GetBincode[T any](db *DB, columnFamily string, key []byte) (*T, error) {
	value, err := db.Get(columnFamily, key)
	if err != nil {
		return nil, err
	}
	return ParseBincode[T](value)
}

func MultiGetBincode[T any](db *DB, columnFamily string, keys ...[]byte) ([]*T, error) {
	values := make([]*T, len(keys))
	for i, key := range keys {
		value, err := GetBincode[T](db, columnFamily, key)
		if err != nil {
			fmt.Printf("cannot decode %s: %s", hex.EncodeToString(key), err)
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}
