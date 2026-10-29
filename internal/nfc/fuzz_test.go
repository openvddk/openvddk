// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package nfc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

// Fuzz targets for every parser of data received from a host (§19, §75).

func FuzzParseAIOHeader(f *testing.F) {
	h := make([]byte, 16)
	binary.LittleEndian.PutUint32(h, aioMagic)
	f.Add(h)
	f.Add([]byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, b []byte) {
		typ, size, op, err := ParseAIOHeader(b)
		if err == nil && (len(b) < 16 || binary.LittleEndian.Uint32(b) != aioMagic) {
			t.Fatalf("accepted %x as type %d size %d op %d", b, typ, size, op)
		}
	})
}

func FuzzParseAllocationReply(f *testing.F) {
	r := make([]byte, 48)
	binary.LittleEndian.PutUint64(r[32:], 100)
	binary.LittleEndian.PutUint32(r[44:], 4)
	f.Add(r, uint64(100))
	f.Fuzz(func(t *testing.T, b []byte, n uint64) {
		w, err := ParseAllocationReply(b, n)
		if err == nil && w != (n+31)/32 {
			t.Fatalf("accepted %d words for %d chunks", w, n)
		}
	})
}

func FuzzParseOpenReply(f *testing.F) {
	r := make([]byte, 60)
	binary.LittleEndian.PutUint32(r[16:], fileTypeDisk)
	f.Add(r)
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = ParseOpenReply(b) })
}

func FuzzRuns(f *testing.F) {
	f.Add([]byte{0xff, 0x01, 0x80}, uint64(24))
	f.Fuzz(func(t *testing.T, bm []byte, n uint64) {
		n %= 1 << 16
		var prev uint64
		for i, r := range Runs(bm, n) {
			if r[0] >= r[1] || r[1] > n || (i > 0 && r[0] <= prev) {
				t.Fatalf("bad run %v after %d (n=%d)", r, prev, n)
			}
			prev = r[1]
		}
	})
}

func FuzzReadLineRaw(f *testing.F) {
	f.Add([]byte("220 hello\r\nrest"))
	f.Fuzz(func(t *testing.T, b []byte) {
		r := bytes.NewReader(b)
		l, err := readLineRaw(r)
		if err == nil && len(l) > maxLineSize {
			t.Fatalf("line of %d bytes", len(l))
		}
	})
}

func FuzzParseIOPiece(f *testing.F) {
	r := make([]byte, ioPayloadSize)
	binary.LittleEndian.PutUint32(r[24:], 1<<20)
	binary.LittleEndian.PutUint32(r[32:], 65536)
	f.Add(r, uint64(0), uint32(1<<20))
	f.Fuzz(func(t *testing.T, b []byte, off uint64, n uint32) {
		po, pl, err := ParseIOPiece(b, off, n)
		if err == nil && (pl == 0 || uint64(po)+uint64(pl) > uint64(n) || po%sectorSize != 0 || pl%sectorSize != 0) {
			t.Fatalf("accepted piece %d+%d of %d", po, pl, n)
		}
	})
}

func FuzzParseDDBReply(f *testing.F) {
	r := make([]byte, ddbPayloadSize)
	binary.LittleEndian.PutUint32(r[12:], 8)
	f.Add(r)
	f.Fuzz(func(t *testing.T, b []byte) {
		n, err := ParseDDBReply(b)
		if err == nil && n > maxDDBValue {
			t.Fatalf("accepted a value of %d bytes", n)
		}
	})
}

func FuzzDecodeSkipZeros(f *testing.F) {
	f.Add([]byte{64, 0, 0, 0, 0, 0, 0, 0, 8, 0, 0, 0, 8, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}, uint16(64))
	f.Fuzz(func(t *testing.T, src []byte, n uint16) {
		dst := make([]byte, n)
		if err := decodeSkipZeros(dst, src); err != nil && !errors.Is(err, ErrProtocol) {
			t.Fatalf("%v is not ErrProtocol", err)
		}
		if len(dst) != int(n) {
			t.Fatal("piece length changed")
		}
	})
}
