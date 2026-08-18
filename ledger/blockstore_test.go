//go:build !lite

package ledger

import (
	"testing"

	"github.com/cockroachdb/pebble/v2"
	"github.com/stretchr/testify/require"
)

func TestPebbleColumnFamilyIsolationAndIteration(t *testing.T) {
	db, err := OpenReadWrite(t.TempDir())
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.Set(CfMeta, []byte("key"), []byte("meta-value")))
	require.NoError(t, db.Set(CfRoot, []byte("key"), []byte("root-value")))

	value, err := db.Get(CfMeta, []byte("key"))
	require.NoError(t, err)
	require.Equal(t, []byte("meta-value"), value)

	value, err = db.Get(CfRoot, []byte("key"))
	require.NoError(t, err)
	require.Equal(t, []byte("root-value"), value)

	iter, err := db.NewIterator(CfMeta)
	require.NoError(t, err)
	defer iter.Close()
	require.True(t, iter.Seek([]byte("key")))
	require.Equal(t, []byte("key"), iter.Key())
	require.Equal(t, []byte("meta-value"), iter.Value())
	require.NoError(t, iter.Error())

	_, err = db.Get(CfMeta, []byte("missing"))
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPebbleReadOnly(t *testing.T) {
	path := t.TempDir()
	db, err := OpenReadWrite(path)
	require.NoError(t, err)
	require.NoError(t, db.Set(CfMeta, []byte("key"), []byte("value")))
	require.NoError(t, db.Close())

	readOnly, err := OpenReadOnly(path)
	require.NoError(t, err)
	defer readOnly.Close()
	require.ErrorIs(t, readOnly.Set(CfMeta, []byte("key"), []byte("new-value")), pebble.ErrReadOnly)
}
