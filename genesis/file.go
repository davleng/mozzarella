package genesis

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	bin "github.com/gagliardetto/binary"
)

// ReadGenesisFromFile is a convenience wrapper for ReadGenesisFromArchive.
func ReadGenesisFromFile(fpath string) (genesis *Genesis, hash *[32]byte, err error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	return ReadGenesisFromArchive(f)
}

// ReadGenesisFromArchive reads a `genesis.tar.bz2` file.
func ReadGenesisFromArchive(archive io.Reader) (genesis *Genesis, hash *[32]byte, err error) {
	var files *tar.Reader
	var hdr *tar.Header
	files, err = OpenTar(archive)
	if err != nil {
		return
	}
	hdr, err = files.Next()
	if err != nil {
		return
	}
	if hdr.Name != "genesis.bin" {
		err = fmt.Errorf("first file is not genesis.bin")
		return
	}

	// Read and hash first file
	const maxSize = 10_000_001
	var rd io.Reader
	rd = files
	hasher := sha256.New()
	rd = io.TeeReader(rd, hasher)
	rd = io.LimitReader(rd, maxSize)

	// Decode content
	var genesisBytes []byte
	genesisBytes, err = io.ReadAll(rd)
	if err != nil {
		return
	}
	if len(genesisBytes) >= maxSize {
		err = fmt.Errorf("genesis.bin too large")
		return
	}
	genesis = new(Genesis)
	dec := bin.NewBinDecoder(genesisBytes)
	err = dec.Decode(genesis)
	if err == nil {
		if dec.HasRemaining() {
			err = fmt.Errorf("not all of genesis.bin was read (%d bytes remaining)", dec.Remaining())
		} else {
			hash = new([32]byte)
			hasher.Sum(hash[:0])
		}
	}

	return
}

// OpenTar opens a `.tar`, `.tar.gz`, or `.tar.bz2` file.
//
// Peeks the first few bytes in the given reader and auto-detects the file format.
// Returns a tar reader spliced together with a decompressor if necessary.
func OpenTar(rawRd io.Reader) (*tar.Reader, error) {
	rd := bufio.NewReader(rawRd)
	magicBytes, err := rd.Peek(6)
	if err != nil {
		return nil, fmt.Errorf("failed to detect magic: %w", err)
	}
	uncompressedRd := io.Reader(rd)

	// Check first few bytes for known compression magics.
	if bytes.Equal(magicBytes[:2], []byte("BZ")) {
		uncompressedRd = bzip2.NewReader(rd)
	} else if bytes.Equal(magicBytes[:3], []byte{0x1f, 0x8b, 0x08}) {
		uncompressedRd, err = gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("invalid .tar.gz: %w", err)
		}
	} else if bytes.Equal(magicBytes[:6], []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}) {
		return nil, fmt.Errorf(".tar.xz not supported yet")
	} else if bytes.Equal(magicBytes[1:4], []byte{0xb5, 0x2f, 0xfd}) {
		return nil, fmt.Errorf(".tar.zst not supported yet")
	} else {
		// Presumed uncompressed case.
		// Peek and see if we can find a valid tar header.
		peek, err := rd.Peek(1024)
		if err != nil {
			return nil, err
		}
		peekTar := tar.NewReader(bytes.NewReader(peek))
		if _, err = peekTar.Next(); err != nil {
			// Doesn't seem to be a valid tar header, bail.
			return nil, fmt.Errorf("unknown archive format")
		}
	}

	return tar.NewReader(uncompressedRd), nil
}
