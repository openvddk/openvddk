// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package nfc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeHost speaks the host side of a session as NET-0002 records it.
type fakeHost struct {
	t        *testing.T
	cert     tls.Certificate
	thumb    string
	bitmap   []byte // returned for the allocation query
	failOpen bool   // answer OPEN_FILE with an error frame
	gotPath  string
	gotQuery [6]uint64
	gotName  string

	ssl        bool              // expect the TLS service and run NFC inside a second TLS session
	ddb        map[string]string // descriptor keys
	readMode   string            // "", "overlap", "outside", "cut", "error", "silent"
	reads      int               // read requests received
	sessionTok string            // the ticket's session ID ("token-1" when empty)
	esxi67     bool              // behave as ESXi 6.7 (NBD-0005): no SHA-256 in the greeting, NFC version 2

	authTLS   func(*tls.Config) // adjusts the authentication service's TLS (nil: Go's defaults)
	streamTLS func(*tls.Config) // adjusts the NFC stream's TLS
	mu        sync.Mutex
	offers    [][]uint16 // the TLS 1.2 suites of every ClientHello, in order

	sparse       bool   // disk content with zero areas (byteAt)
	skipzUnasked bool   // leave zeros out even when the read did not ask for it
	skipzAsked   int    // read requests asking to leave zeros out (code 3)
	skipzSent    int    // pieces sent with their zeros left out
	wholeSent    int    // pieces sent whole
	gotSession   []byte // the AIO open-session payload; a buffer size in it sets the piece size
	gotSockOpts  []byte // the socket options payload
}

// byteAt is the fake disk's content: diskByte, with zero areas when sparse
// (a whole 64 KiB piece, a sector in eight, 40 bytes inside sectors).
func (f *fakeHost) byteAt(off uint64) byte {
	if f.sparse {
		piece := off >> 16
		switch {
		case piece%4 == 2:
			return 0
		case piece%4 != 0 && (off>>9)%8 == 3:
			return 0
		case piece%4 == 3 && off%512 >= 96 && off%512 < 136:
			return 0
		}
	}
	return diskByte(off)
}

// skipZeros encodes a piece as the host does with code 3 (NET-0004): the
// piece's length and a zero word, then runs of 8-byte words, a zero gap of
// two words or more ending a run; nil when the piece has no such gap (the
// host sends it whole, code 0).
func skipZeros(d []byte) []byte {
	zero := func(i int) bool {
		for _, b := range d[i : i+8] {
			if b != 0 {
				return false
			}
		}
		return true
	}
	var runs [][2]int
	start, end := -1, -1
	for i := 0; i+8 <= len(d); i += 8 {
		if zero(i) {
			continue
		}
		if start >= 0 && i-end >= 16 {
			runs = append(runs, [2]int{start, end})
			start = -1
		}
		if start < 0 {
			start = i
		}
		end = i + 8
	}
	if start >= 0 {
		runs = append(runs, [2]int{start, end})
	}
	if len(runs) == 1 && runs[0] == [2]int{0, len(d)} {
		return nil
	}
	out := binary.LittleEndian.AppendUint32(nil, uint32(len(d)))
	out = binary.LittleEndian.AppendUint32(out, 0)
	for _, r := range runs {
		out = binary.LittleEndian.AppendUint32(out, uint32(r[0]))
		out = binary.LittleEndian.AppendUint32(out, uint32(r[1]-r[0]))
		out = append(out, d[r[0]:r[1]]...)
	}
	return out
}

// tlsConfig is the fake's TLS for one session; it records the client's
// TLS 1.2 offer.
func (f *fakeHost) tlsConfig(adjust func(*tls.Config)) *tls.Config {
	c := &tls.Config{Certificates: []tls.Certificate{f.cert}, GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
		var tls12 []uint16
		for _, s := range h.CipherSuites {
			if s < tls.TLS_AES_128_GCM_SHA256 || s > tls.TLS_CHACHA20_POLY1305_SHA256 {
				tls12 = append(tls12, s)
			}
		}
		f.mu.Lock()
		f.offers = append(f.offers, tls12)
		f.mu.Unlock()
		return nil, nil
	}}
	if adjust != nil {
		adjust(c)
	}
	return c
}

func (f *fakeHost) offered() [][]uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.offers)
}

// diskByte is the content of the fake disk.
func diskByte(off uint64) byte { return byte(off>>9) ^ byte(off*7) }

func newFakeHost(t *testing.T) *fakeHost {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake-host"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	s := sha1.Sum(der)
	return &fakeHost{t: t, cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, thumb: hex.EncodeToString(s[:])}
}

func (f *fakeHost) serve(ln net.Listener, done chan<- error, sessions int) {
	for range sessions {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		done <- f.session(c)
		c.Close()
	}
}

func (f *fakeHost) session(c net.Conn) error {
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	greeting, thumbCmd := "220 test authentication service, TLS required, SHA256 supported\r\n", "THUMBPRINT_SHA2 PlainText"
	if f.esxi67 {
		greeting, thumbCmd = "220 test authentication service, TLS required\r\n", "THUMBPRINT PlainText"
	}
	if _, err := io.WriteString(c, greeting); err != nil {
		return err
	}
	tc := tls.Server(c, f.tlsConfig(f.authTLS))
	if err := tc.Handshake(); err != nil {
		return err
	}
	proxy := "PROXY vpxa-nfc"
	if f.ssl {
		proxy += "ssl"
	}
	want := []struct{ line, reply string }{
		{"SESSION " + f.sessionID(), ""},
		{"BANNER ", "220 test\r\n"},
		{thumbCmd, "200 test\r\n"},
		{proxy, "200 Connect test\r\n"},
	}
	for _, w := range want {
		l, err := readLineRaw(tc) // byte by byte: nothing after the last line is consumed
		if err != nil {
			return err
		}
		if l != w.line {
			return errors.New("unexpected command " + l)
		}
		if w.reply != "" {
			if _, err := io.WriteString(tc, w.reply); err != nil {
				return err
			}
		}
	}
	if f.ssl {
		sc := tls.Server(c, f.tlsConfig(f.streamTLS))
		if err := sc.Handshake(); err != nil {
			return err
		}
		c = sc
	}
	rd := bufio.NewReader(c) // NFC from here on (clear text, or inside the second TLS session)
	next := func(want uint32) ([]byte, error) {
		b := make([]byte, classicSize)
		if _, err := io.ReadFull(rd, b); err != nil {
			return nil, err
		}
		if t := binary.LittleEndian.Uint32(b); t != want {
			return nil, errors.New("unexpected classic message")
		}
		return b, nil
	}
	if b, err := next(msgHandshake); err != nil || !strings.HasPrefix(string(b[4:]), "PlainText") {
		return errors.New("bad handshake")
	}
	if _, err := next(msgSessionParams); err != nil {
		return err
	}
	r := classic(msgSessionParamsReply)
	binary.LittleEndian.PutUint32(r[16:], 1)
	c.Write(r)
	if _, err := next(msgVersion); err != nil {
		return err
	}
	v := classic(msgVersion)
	if f.esxi67 {
		binary.LittleEndian.PutUint32(v[4:], 2)
	} else {
		binary.LittleEndian.PutUint32(v[4:], 9)
	}
	c.Write(v)
	if !f.esxi67 {
		cd, err := next(msgConnectionData)
		if err != nil {
			return err
		}
		nl, ol := binary.LittleEndian.Uint32(cd[4:]), binary.LittleEndian.Uint32(cd[8:])
		names := make([]byte, nl+ol)
		if _, err := io.ReadFull(rd, names); err != nil {
			return err
		}
		f.gotName = string(names[:nl])
		if _, err := next(msgFeatures); err != nil {
			return err
		}
	}
	if _, err := next(msgAIOSession); err != nil {
		return err
	}
	c.Write(classic(msgAIOSession))
	for {
		h := make([]byte, aioHeaderSize)
		if _, err := io.ReadFull(rd, h); err != nil {
			return err
		}
		typ, size, op, err := ParseAIOHeader(h)
		if err != nil {
			return err
		}
		p := make([]byte, size)
		if _, err := io.ReadFull(rd, p); err != nil {
			return err
		}
		reply := func(t uint32, payload, extra []byte) {
			rh := make([]byte, aioHeaderSize)
			binary.LittleEndian.PutUint32(rh[0:], aioMagic)
			binary.LittleEndian.PutUint32(rh[4:], t)
			binary.LittleEndian.PutUint32(rh[8:], uint32(len(payload)))
			binary.LittleEndian.PutUint32(rh[12:], op)
			c.Write(append(append(rh, payload...), extra...))
		}
		switch typ {
		case aioOpenSession:
			f.gotSession = append([]byte(nil), p...)
			reply(typ, make([]byte, 16), nil)
		case aioSockOpts:
			f.gotSockOpts = append([]byte(nil), p...)
			reply(typ, make([]byte, 12), nil)
		case aioResPool:
			if f.esxi67 {
				return errors.New("resource pool request to a version 2 host")
			}
			reply(typ, make([]byte, 4), nil)
		case aioOpenFile:
			path := make([]byte, binary.LittleEndian.Uint32(p[0:]))
			if _, err := io.ReadFull(rd, path); err != nil {
				return err
			}
			f.gotPath = string(path)
			if f.failOpen {
				reply(aioError, make([]byte, 8), nil)
				continue
			}
			rp := make([]byte, 60)
			binary.LittleEndian.PutUint64(rp[8:], 0x71b14d0)
			binary.LittleEndian.PutUint32(rp[16:], fileTypeDisk)
			binary.LittleEndian.PutUint32(rp[20:], openFlagsReadOnly)
			reply(typ, rp, nil)
		case aioAllocation:
			for i := range f.gotQuery {
				f.gotQuery[i] = binary.LittleEndian.Uint64(p[i*8:])
			}
			n := f.gotQuery[4]
			rp := make([]byte, 48)
			binary.LittleEndian.PutUint64(rp[32:], n)
			words := (n + 31) / 32
			binary.LittleEndian.PutUint32(rp[44:], uint32(words))
			bm := make([]byte, words*4)
			copy(bm, f.bitmap)
			reply(typ, rp, bm)
		case aioDDBGet:
			key := make([]byte, binary.LittleEndian.Uint32(p[8:]))
			if _, err := io.ReadFull(rd, key); err != nil {
				return err
			}
			v := f.ddb[string(key)]
			rp := make([]byte, ddbPayloadSize)
			binary.LittleEndian.PutUint32(rp[12:], uint32(len(v)))
			reply(typ, rp, []byte(v))
		case aioIO:
			f.reads++
			off, n := binary.LittleEndian.Uint64(p[16:]), binary.LittleEndian.Uint32(p[24:])
			askZ := binary.LittleEndian.Uint32(p[12:]) == ioSkipZeros
			if askZ {
				f.skipzAsked++
			}
			piece := uint32(64 * 1024) // the host's default buffer
			if len(f.gotSession) >= 8 && binary.LittleEndian.Uint32(f.gotSession[4:]) > 0 {
				piece = binary.LittleEndian.Uint32(f.gotSession[4:])
			}
			var offs []uint32
			for o := uint32(0); o < n; o += piece {
				offs = append(offs, o)
			}
			for i := 0; i+1 < len(offs); i += 2 { // the host's pieces arrive in swapped pairs (NET-0003)
				offs[i], offs[i+1] = offs[i+1], offs[i]
			}
			switch f.readMode {
			case "error":
				reply(aioError, make([]byte, 8), nil)
				continue
			case "silent":
				continue
			case "overlap": // the first piece again, before the request is whole
				offs = append([]uint32{offs[0], offs[0]}, offs[1:]...)
			case "late": // the first piece again, after the request is whole
				offs = append(offs, offs[0])
			case "outside":
				offs[0] = n
			}
			for i, po := range offs {
				pl := min(piece, n-min(po, n))
				if f.readMode == "outside" && i == 0 {
					pl = piece
				}
				rp := make([]byte, ioPayloadSize)
				binary.LittleEndian.PutUint32(rp[8:], 1)
				binary.LittleEndian.PutUint64(rp[16:], off)
				binary.LittleEndian.PutUint32(rp[24:], n)
				binary.LittleEndian.PutUint32(rp[28:], po)
				binary.LittleEndian.PutUint32(rp[32:], pl)
				binary.LittleEndian.PutUint32(rp[36:], pl)
				data := make([]byte, pl)
				for j := range data {
					data[j] = f.byteAt(off + uint64(po) + uint64(j))
				}
				if askZ || f.skipzUnasked {
					if z := skipZeros(data); z != nil {
						binary.LittleEndian.PutUint32(rp[12:], ioSkipZeros)
						binary.LittleEndian.PutUint32(rp[36:], uint32(len(z)))
						data = z
						f.skipzSent++
					} else {
						f.wholeSent++
					}
				} else {
					f.wholeSent++
				}
				if f.readMode == "cut" && i == 1 {
					reply(typ, rp, data[:pl/2])
					return nil
				}
				reply(typ, rp, data)
			}
		case aioCloseFile:
			reply(typ, make([]byte, 8), nil)
		case aioCloseSession:
			reply(typ, make([]byte, 4), nil)
			c.Write(classic(msgSessionComplete))
			_, err := next(msgSessionComplete)
			return err
		default:
			return errors.New("unexpected AIO request")
		}
	}
}

func startFake(t *testing.T, f *fakeHost) (Ticket, string, chan error) {
	return startFakeN(t, f, 1)
}

// startFakeN serves that many sessions, one after the other.
func startFakeN(t *testing.T, f *fakeHost, sessions int) (Ticket, string, chan error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	done := make(chan error, sessions)
	go f.serve(ln, done, sessions)
	addr := ln.Addr().(*net.TCPAddr)
	return Ticket{Host: "127.0.0.1", Port: addr.Port, SSLThumbprint: f.thumb, Service: "vpxa-nfc", SessionID: f.sessionID()}, "127.0.0.1", done
}

func (f *fakeHost) sessionID() string {
	if f.sessionTok == "" {
		return "token-1"
	}
	return f.sessionTok
}

// A session with ESXi 6.7 as the reference holds it (NBD-0005): vCenter 6.7
// gives the session ID in VMware's spaced UUID form, sent as it is; the
// greeting has no "SHA256 supported", so THUMBPRINT is asked; the host
// answers NFC version 2, so no connection data, features or resource pool.
// A line break in the session ID is refused before anything is sent.
func TestESXi67Session(t *testing.T) {
	f := newFakeHost(t)
	f.sessionTok = "52 1a 2b 3c 4d 5e 6f 70-81 92 a3 b4 c5 d6 e7 f8"
	f.esxi67 = true
	tk, addr, done := startFake(t, f)
	c, err := Dial(context.Background(), addr, tk, Options{Timeout: 10 * time.Second, Operation: "nbdmode"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 2 {
		t.Fatalf("version %d", c.Version)
	}
	h, err := c.OpenDisk("[ds] vm/vm.vmdk")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 128<<10)
	if err := c.Read(h, 1<<20, buf); err != nil {
		t.Fatal(err)
	}
	for i, b := range buf {
		if b != diskByte(1<<20+uint64(i)) {
			t.Fatalf("byte %d", i)
		}
	}
	if err := c.CloseDisk(h); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("host side: %v", err)
	}
	g := newFakeHost(t)
	tk, addr, _ = startFake(t, g)
	tk.SessionID = "token-1\r\nPROXY other"
	if _, err := Dial(context.Background(), addr, tk, Options{Timeout: 5 * time.Second}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("line break in the session ID: got %v, want ErrProtocol", err)
	}
}

func TestSessionAllocation(t *testing.T) {
	f := newFakeHost(t)
	// bits 0..31, 40, 41 and 99 of 100 chunks
	f.bitmap = make([]byte, 16)
	for i := 0; i < 4; i++ {
		f.bitmap[i] = 0xff
	}
	f.bitmap[5] = 0x03
	f.bitmap[12] = 0x08
	tk, addr, done := startFake(t, f)
	c, err := Dial(context.Background(), addr, tk, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 9 {
		t.Fatalf("version %d", c.Version)
	}
	h, err := c.OpenDisk("[ds] vm/vm.vmdk")
	if err != nil {
		t.Fatal(err)
	}
	bm, err := c.Allocation(h, 65536, 1<<30, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.CloseDisk(h); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("host side: %v", err)
	}
	if f.gotPath != "[ds] vm/vm.vmdk" || f.gotName != "openvddk" {
		t.Fatalf("path %q name %q", f.gotPath, f.gotName)
	}
	if q := f.gotQuery; q[0] != 0x71b14d0 || q[2] != 65536 || q[3] != 1<<30 || q[4] != 100 {
		t.Fatalf("query %#x", q)
	}
	got := Runs(bm, 100)
	want := [][2]uint64{{0, 32}, {40, 42}, {99, 100}}
	if len(got) != len(want) {
		t.Fatalf("runs %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("runs %v, want %v", got, want)
		}
	}
}

func TestOpenRefused(t *testing.T) {
	f := newFakeHost(t)
	f.failOpen = true
	tk, addr, _ := startFake(t, f)
	c, err := Dial(context.Background(), addr, tk, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer c.c.Close()
	if _, err := c.OpenDisk("[ds] missing.vmdk"); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v, want ErrRefused", err)
	}
}

func TestWrongThumbprint(t *testing.T) {
	f := newFakeHost(t)
	tk, addr, _ := startFake(t, f)
	tk.SSLThumbprint = strings.Repeat("00", 20)
	if _, err := Dial(context.Background(), addr, tk, Options{Timeout: 5 * time.Second}); err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("got %v, want a TLS failure", err)
	}
}

func TestParseAllocationReply(t *testing.T) {
	r := make([]byte, 48)
	binary.LittleEndian.PutUint64(r[32:], 327680)
	binary.LittleEndian.PutUint32(r[44:], 10240)
	if w, err := ParseAllocationReply(r, 327680); err != nil || w != 10240 {
		t.Fatalf("w=%d err=%v", w, err)
	}
	if _, err := ParseAllocationReply(r, 320); err == nil {
		t.Fatal("count mismatch accepted")
	}
	binary.LittleEndian.PutUint32(r[44:], 0xffffffff)
	if _, err := ParseAllocationReply(r, 327680); err == nil {
		t.Fatal("oversized bitmap accepted")
	}
}

func TestRuns(t *testing.T) {
	if r := Runs(nil, 10); len(r) != 0 {
		t.Fatalf("runs of empty bitmap: %v", r)
	}
	if r := Runs([]byte{0xff, 0xff}, 12); len(r) != 1 || r[0] != [2]uint64{0, 12} {
		t.Fatalf("runs cut at n: %v", r)
	}
	if r := Runs([]byte{0x80}, 100); len(r) != 1 || r[0] != [2]uint64{7, 8} {
		t.Fatalf("short bitmap: %v", r)
	}
}

// openRead dials the fake host, opens a disk and returns the session and
// handle.
func openRead(t *testing.T, f *fakeHost, o Options) (*Conn, uint64, chan error) {
	t.Helper()
	tk, addr, done := startFake(t, f)
	if o.Timeout == 0 {
		o.Timeout = 10 * time.Second
	}
	c, err := Dial(context.Background(), addr, tk, o)
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.OpenDisk("[ds] vm/vm.vmdk")
	if err != nil {
		t.Fatal(err)
	}
	return c, h, done
}

func checkData(t *testing.T, buf []byte, off uint64) {
	t.Helper()
	for i := range buf {
		if buf[i] != diskByte(off+uint64(i)) {
			t.Fatalf("byte %d of the read (disk offset %d) is %#x, want %#x", i, off+uint64(i), buf[i], diskByte(off+uint64(i)))
		}
	}
}

// A read spanning several requests, the last one partial, comes back whole
// although the host sends the pieces out of order; descriptor keys too.
func testReadSession(t *testing.T, ssl bool) {
	f := newFakeHost(t)
	f.ssl = ssl
	f.ddb = map[string]string{"adapterType": "lsilogic", "geometry.cylinders": "33288"}
	c, h, done := openRead(t, f, Options{SSL: ssl, InFlight: 3})
	buf := make([]byte, 3*IOChunk+512)
	if err := c.Read(h, 1<<20, buf); err != nil {
		t.Fatal(err)
	}
	checkData(t, buf, 1<<20)
	if f.reads != 4 {
		t.Fatalf("%d read requests, want 4", f.reads)
	}
	if v, ok, err := c.DDBGet(h, "adapterType"); err != nil || !ok || v != "lsilogic" {
		t.Fatalf("adapterType %q %v %v", v, ok, err)
	}
	if v, ok, err := c.DDBGet(h, "uuid"); err != nil || ok || v != "" {
		t.Fatalf("missing key gave %q %v %v", v, ok, err)
	}
	if err := c.CloseDisk(h); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("host side: %v", err)
	}
}

func TestReadNBD(t *testing.T)    { testReadSession(t, false) }
func TestReadNBDSSL(t *testing.T) { testReadSession(t, true) }

// Pieces the host should never send end the session; the read fails and
// later requests fail too.
func TestReadProtocolViolations(t *testing.T) {
	for _, mode := range []string{"overlap", "outside", "cut"} {
		t.Run(mode, func(t *testing.T) {
			f := newFakeHost(t)
			f.readMode = mode
			c, h, _ := openRead(t, f, Options{})
			buf := make([]byte, 256*1024)
			err := c.Read(h, 0, buf)
			if err == nil {
				t.Fatal("read succeeded")
			}
			if mode != "cut" && !errors.Is(err, ErrProtocol) {
				t.Fatalf("got %v, want ErrProtocol", err)
			}
			if _, _, err := c.DDBGet(h, "uuid"); err == nil {
				t.Fatal("session still usable after a protocol violation")
			}
			c.Close()
		})
	}
}

// A piece after its request was whole ends the session: the read had all
// its bytes exactly once, but nothing later is trusted.
func TestLatePieceEndsSession(t *testing.T) {
	f := newFakeHost(t)
	f.readMode = "late"
	c, h, _ := openRead(t, f, Options{})
	buf := make([]byte, 256*1024)
	if err := c.Read(h, 0, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	checkData(t, buf, 0)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, _, err := c.DDBGet(h, "uuid")
		if errors.Is(err, ErrProtocol) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session still usable after a late piece: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.Close()
}

// An error reply fails that read only; the session goes on.
func TestReadErrorReply(t *testing.T) {
	f := newFakeHost(t)
	f.readMode = "error"
	f.ddb = map[string]string{"uuid": "60 00 c2 9f"}
	c, h, _ := openRead(t, f, Options{})
	if err := c.Read(h, 0, make([]byte, 4096)); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v, want ErrRefused", err)
	}
	if v, ok, err := c.DDBGet(h, "uuid"); err != nil || !ok || v != "60 00 c2 9f" {
		t.Fatalf("after the error reply: %q %v %v", v, ok, err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

// A host that stops answering fails the read after the timeout.
func TestReadTimeout(t *testing.T) {
	f := newFakeHost(t)
	f.readMode = "silent"
	c, h, _ := openRead(t, f, Options{Timeout: 300 * time.Millisecond})
	t0 := time.Now()
	err := c.Read(h, 0, make([]byte, 4096))
	if err == nil {
		t.Fatal("read succeeded")
	}
	if d := time.Since(t0); d > 5*time.Second {
		t.Fatalf("read failed after %v", d)
	}
	c.Close()
}

// An idle session stays open longer than the timeout.
func TestIdleSession(t *testing.T) {
	f := newFakeHost(t)
	f.ddb = map[string]string{"uuid": "x"}
	c, h, _ := openRead(t, f, Options{Timeout: 200 * time.Millisecond})
	time.Sleep(600 * time.Millisecond)
	if _, ok, err := c.DDBGet(h, "uuid"); err != nil || !ok {
		t.Fatalf("after idling: %v %v", ok, err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestParseIOPiece(t *testing.T) {
	r := make([]byte, ioPayloadSize)
	binary.LittleEndian.PutUint64(r[16:], 1<<20)
	binary.LittleEndian.PutUint32(r[24:], 1<<20)
	binary.LittleEndian.PutUint32(r[28:], 65536)
	binary.LittleEndian.PutUint32(r[32:], 65536)
	if o, l, err := ParseIOPiece(r, 1<<20, 1<<20); err != nil || o != 65536 || l != 65536 {
		t.Fatalf("o=%d l=%d err=%v", o, l, err)
	}
	for _, bad := range []struct{ at, v int }{{28, 1 << 20}, {32, 100}, {28, 1<<20 - 512}, {32, 0}} {
		b := append([]byte(nil), r...)
		binary.LittleEndian.PutUint32(b[bad.at:], uint32(bad.v))
		if bad.at == 28 && bad.v == 1<<20-512 {
			binary.LittleEndian.PutUint32(b[32:], 65536) // runs past the end of the request
		}
		if _, _, err := ParseIOPiece(b, 1<<20, 1<<20); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	if _, _, err := ParseIOPiece(r, 0, 1<<20); err == nil {
		t.Error("accepted a reply for another offset")
	}
}

// An allocation query to ESXi 6.7 as the reference holds the session: the
// spaced session ID sent as it is, THUMBPRINT because the greeting has no
// "SHA256 supported", no connection data, features or resource pool for an
// NFC version 2 host. A line break in the session ID is refused.
func TestESXi67Allocation(t *testing.T) {
	f := newFakeHost(t)
	f.esxi67 = true
	f.bitmap = []byte{0x0f}
	tk, addr, done := startFake(t, f)
	c, err := Dial(context.Background(), addr, tk, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 2 {
		t.Fatalf("version %d", c.Version)
	}
	h, err := c.OpenDisk("[ds] vm/vm.vmdk")
	if err != nil {
		t.Fatal(err)
	}
	bm, err := c.Allocation(h, 65536, 1<<30, 8)
	if err != nil {
		t.Fatal(err)
	}
	if got := Runs(bm, 8); len(got) != 1 || got[0] != [2]uint64{0, 4} {
		t.Fatalf("runs %v", got)
	}
	if err := c.CloseDisk(h); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("host side: %v", err)
	}
	g := newFakeHost(t)
	tk, addr, _ = startFake(t, g)
	tk.SessionID = "token-1\r\nPROXY other"
	if _, err := Dial(context.Background(), addr, tk, Options{Timeout: 5 * time.Second}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("line break in the session ID: got %v, want ErrProtocol", err)
	}
}

// tls12Only lets a fake TLS endpoint accept TLS 1.2 with these suites only,
// as ESXi 6.7 and 7.0 accept TLS 1.2 alone (NBD-0023).
func tls12Only(suites ...uint16) func(*tls.Config) {
	return func(c *tls.Config) { c.MaxVersion, c.CipherSuites = tls.VersionTLS12, suites }
}

func withAESHardware(t *testing.T, on bool) {
	old := aesHardware
	aesHardware = on
	t.Cleanup(func() { aesHardware = old })
}

const (
	ecdsaAES128 = tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
	ecdsaAES256 = tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
)

// On a CPU without AES instructions both TLS sessions of an NBDSSL session
// offer AES-128-GCM alone over TLS 1.2, so that a host choosing AES-256-GCM
// whenever it is offered agrees on AES-128-GCM (NBD-0023); with the
// instructions, and with AllCiphers, the offer is Go's full list.
func TestAES128OfferWithoutAESHardware(t *testing.T) {
	for _, c := range []struct {
		hw, all bool
		only128 bool
	}{{false, false, true}, {true, false, false}, {false, true, false}} {
		withAESHardware(t, c.hw)
		f := newFakeHost(t)
		f.ssl = true
		f.authTLS, f.streamTLS = tls12Only(ecdsaAES256, ecdsaAES128), tls12Only(ecdsaAES256, ecdsaAES128)
		tk, addr, _ := startFake(t, f)
		conn, err := Dial(context.Background(), addr, tk, Options{SSL: true, AllCiphers: c.all, Timeout: 10 * time.Second})
		if err != nil {
			t.Fatalf("AES instructions %v, AllCiphers %v: %v", c.hw, c.all, err)
		}
		conn.c.Close()
		offers := f.offered()
		if len(offers) != 2 {
			t.Fatalf("AES instructions %v, AllCiphers %v: %d ClientHellos, want 2", c.hw, c.all, len(offers))
		}
		for i, o := range offers {
			if got := onlyAES128(o); got != c.only128 {
				t.Errorf("AES instructions %v, AllCiphers %v, session %d: offer %#x, AES-128-GCM alone %v, want %v", c.hw, c.all, i+1, o, got, c.only128)
			}
		}
		if c.only128 && conn.TLS != "TLS 1.2, TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256" {
			t.Errorf("negotiated %q", conn.TLS)
		}
	}
}

// An authentication service that refuses AES-128-GCM alone: Dial connects
// again with the full list (the ticket was not presented yet) and succeeds.
func TestAuthServiceRefusingAES128(t *testing.T) {
	withAESHardware(t, false)
	f := newFakeHost(t)
	f.authTLS = tls12Only(ecdsaAES256)
	tk, addr, done := startFakeN(t, f, 2)
	conn, err := Dial(context.Background(), addr, tk, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.c.Close()
	if conn.TLS != "TLS 1.2, TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384" {
		t.Errorf("negotiated %q", conn.TLS)
	}
	if err := <-done; err == nil {
		t.Error("the first session should have failed at the TLS handshake")
	}
	if offers := f.offered(); len(offers) != 2 || !onlyAES128(offers[0]) || onlyAES128(offers[1]) {
		t.Errorf("offers %#x, want AES-128-GCM alone, then the full list", offers)
	}
}

// A host that refuses AES-128-GCM alone for the NFC stream, after the
// ticket was presented: ErrCipherRefused, never ErrStreamTLS (which could
// let an NBDSSL open go on to clear-text NBD); with AllCiphers (and, in the
// library, a new ticket) the stream opens.
func TestStreamRefusingAES128(t *testing.T) {
	withAESHardware(t, false)
	for _, all := range []bool{false, true} {
		f := newFakeHost(t)
		f.ssl = true
		f.streamTLS = tls12Only(ecdsaAES256)
		tk, addr, _ := startFake(t, f)
		conn, err := Dial(context.Background(), addr, tk, Options{SSL: true, AllCiphers: all, Timeout: 10 * time.Second})
		if !all {
			if !errors.Is(err, ErrCipherRefused) || errors.Is(err, ErrStreamTLS) {
				t.Fatalf("got %v, want ErrCipherRefused and not ErrStreamTLS", err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("AllCiphers: %v", err)
		}
		conn.c.Close()
		if conn.TLS != "TLS 1.2, TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384" {
			t.Errorf("negotiated %q", conn.TLS)
		}
	}
}

// onlyAES128 reports whether a TLS 1.2 offer is aes128Suites (Go orders the
// suites itself).
func onlyAES128(offer []uint16) bool {
	a, b := slices.Clone(offer), slices.Clone(aes128Suites)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// Reads that ask the host to leave zeros out (the consumer opened with
// skipz): the requests carry code 3, the host sends zero-skipped pieces and
// whole ones, and the data is the disk's; without the flag, code 0 and
// whole pieces only (NET-0004).
func TestReadSkipZeros(t *testing.T) {
	for _, skip := range []bool{true, false} {
		f := newFakeHost(t)
		f.sparse = true
		c, h, done := openRead(t, f, Options{SkipZeros: skip, InFlight: 4})
		const off = 3 << 20
		buf := make([]byte, 4*IOChunk)
		for i := range buf {
			buf[i] = 0xEE // the zeros must come from the decoding
		}
		if err := c.Read(h, off, buf); err != nil {
			t.Fatalf("skipz %v: %v", skip, err)
		}
		for i := range buf {
			if want := f.byteAt(off + uint64(i)); buf[i] != want {
				t.Fatalf("skipz %v: byte %d is %#x, want %#x", skip, i, buf[i], want)
			}
		}
		if err := c.CloseDisk(h); err != nil {
			t.Fatal(err)
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatalf("host side: %v", err)
		}
		if skip && (f.skipzAsked != 4 || f.skipzSent == 0 || f.wholeSent == 0) {
			t.Errorf("skipz: %d requests asked, %d pieces zero-skipped, %d whole", f.skipzAsked, f.skipzSent, f.wholeSent)
		}
		if !skip && (f.skipzAsked != 0 || f.skipzSent != 0) {
			t.Errorf("without skipz: %d requests asked, %d pieces zero-skipped", f.skipzAsked, f.skipzSent)
		}
	}
}

// A zero-skipped piece the read did not ask for ends the session with a
// protocol error instead of being read as data.
func TestUnaskedSkipZeros(t *testing.T) {
	f := newFakeHost(t)
	f.sparse = true
	f.skipzUnasked = true
	c, h, _ := openRead(t, f, Options{})
	defer c.c.Close()
	if err := c.Read(h, 2<<20, make([]byte, IOChunk)); !errors.Is(err, ErrProtocol) {
		t.Fatalf("got %v, want ErrProtocol", err)
	}
}

func TestDecodeSkipZeros(t *testing.T) {
	enc := func(n uint32, words ...uint32) []byte {
		b := binary.LittleEndian.AppendUint32(nil, n)
		b = binary.LittleEndian.AppendUint32(b, 0)
		for _, w := range words {
			b = binary.LittleEndian.AppendUint32(b, w)
		}
		return b
	}
	run := func(b []byte, off, n uint32, fill byte) []byte {
		b = binary.LittleEndian.AppendUint32(b, off)
		b = binary.LittleEndian.AppendUint32(b, n)
		return append(b, bytes.Repeat([]byte{fill}, int(n))...)
	}
	two := run(run(enc(64), 0, 8, 1), 32, 16, 2)
	for _, c := range []struct {
		name string
		n    int
		src  []byte
		ok   bool
	}{
		{"all zeros", 64, enc(64), true},
		{"two runs", 64, two, true},
		{"run to the end", 64, run(enc(64), 56, 8, 3), true},
		{"empty", 64, nil, false},
		{"header cut short", 64, enc(64)[:6], false},
		{"length of another piece", 64, enc(128), false},
		{"second word not zero", 64, append(binary.LittleEndian.AppendUint32(nil, 64), 1, 0, 0, 0), false},
		{"run past the end", 64, run(enc(64), 60, 8, 1), false},
		{"runs out of order", 64, run(run(enc(64), 32, 8, 1), 0, 8, 2), false},
		{"runs overlapping", 64, run(run(enc(64), 0, 16, 1), 8, 8, 2), false},
		{"empty run", 64, run(enc(64), 8, 0, 1), false},
		{"run data cut short", 64, run(enc(64), 0, 16, 1)[:20], false},
		{"run header cut short", 64, append(enc(64), 1, 2, 3), false},
	} {
		dst := bytes.Repeat([]byte{0xEE}, c.n)
		err := decodeSkipZeros(dst, c.src)
		if (err == nil) != c.ok {
			t.Errorf("%s: error %v, want ok %v", c.name, err, c.ok)
			continue
		}
		if err != nil && !errors.Is(err, ErrProtocol) {
			t.Errorf("%s: %v is not ErrProtocol", c.name, err)
		}
	}
	dst := bytes.Repeat([]byte{0xEE}, 64)
	if err := decodeSkipZeros(dst, two); err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 64)
	copy(want[0:8], bytes.Repeat([]byte{1}, 8))
	copy(want[32:48], bytes.Repeat([]byte{2}, 16))
	if !bytes.Equal(dst, want) {
		t.Fatalf("decoded %x, want %x", dst, want)
	}
	// The fake host's encoder and the decoder agree on the sparse content.
	f := &fakeHost{sparse: true}
	for piece := uint64(0); piece < 8; piece++ {
		d := make([]byte, 65536)
		for i := range d {
			d[i] = f.byteAt(piece<<16 + uint64(i))
		}
		z := skipZeros(d)
		if z == nil {
			continue
		}
		got := make([]byte, len(d))
		if err := decodeSkipZeros(got, z); err != nil || !bytes.Equal(got, d) {
			t.Fatalf("piece %d: round trip %v", piece, err)
		}
	}
}

func TestParseIOEncoding(t *testing.T) {
	piece := func(code, wire uint32) []byte {
		r := make([]byte, ioPayloadSize)
		binary.LittleEndian.PutUint32(r[12:], code)
		binary.LittleEndian.PutUint32(r[36:], wire)
		return r
	}
	for _, c := range []struct {
		code, wire uint32
		asked, ok  bool
		skip       bool
	}{
		{0, 65536, false, true, false},
		{0, 65536, true, true, false}, // pieces without zeros come whole
		{0, 0, false, true, false},    // code 0: the piece length, as before skipz
		{3, 8, true, true, true},
		{3, 40000, true, true, true},
		{3, 8, false, false, false}, // not asked for
		{3, 4, true, false, false},
		{3, 1 << 20, true, false, false},
		{1, 100, true, false, false},
	} {
		wire, skip, err := ParseIOEncoding(piece(c.code, c.wire), 65536, c.asked)
		want := c.wire
		if c.code == 0 {
			want = 65536
		}
		if (err == nil) != c.ok || (err == nil && (skip != c.skip || wire != want)) {
			t.Errorf("code %d wire %d asked %v: %d %v %v", c.code, c.wire, c.asked, wire, skip, err)
		}
	}
}

// The AIO session carries the buffer size and count VMware's
// vixDiskLib.nfcAio.Session keys set (16 zero bytes without them, as the
// reference sends); the host then sends reads in pieces of that size
// (D-0048). A read comes back whole either way.
func TestAIOSessionBuffers(t *testing.T) {
	for _, c := range []struct {
		name        string
		size, count uint32
		wantPieces  int
	}{
		{"host default", 0, 0, 49},          // 3 MiB + 512 in 64 KiB pieces
		{"2 MiB, 4 buffers", 2 << 20, 4, 4}, // one piece per 1 MiB request
	} {
		f := newFakeHost(t)
		c := c
		conn, h, done := openRead(t, f, Options{AIOBufSize: c.size, AIOBufCount: c.count})
		buf := make([]byte, 3*IOChunk+512)
		if err := conn.Read(h, 1<<20, buf); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		checkData(t, buf, 1<<20)
		if err := conn.CloseDisk(h); err != nil {
			t.Fatal(err)
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatalf("%s: host side: %v", c.name, err)
		}
		want := make([]byte, 16)
		binary.LittleEndian.PutUint32(want[4:], c.size)
		binary.LittleEndian.PutUint32(want[8:], c.count)
		if !bytes.Equal(f.gotSession, want) {
			t.Errorf("%s: open-session payload %x, want %x", c.name, f.gotSession, want)
		}
		if f.wholeSent != c.wantPieces {
			t.Errorf("%s: %d pieces sent, want %d", c.name, f.wholeSent, c.wantPieces)
		}
	}
}

// VMware's vixDiskLib.nfc.ReadTimeoutMs bounds the wait for a reply once
// the session runs, separately from the per-step timeout (D-0049).
func TestReadTimeoutSetting(t *testing.T) {
	f := newFakeHost(t)
	f.readMode = "silent"
	c, h, _ := openRead(t, f, Options{Timeout: 30 * time.Second, ReadTimeout: 300 * time.Millisecond})
	t0 := time.Now()
	if err := c.Read(h, 0, make([]byte, 4096)); err == nil {
		t.Fatal("read succeeded")
	}
	if d := time.Since(t0); d > 5*time.Second {
		t.Fatalf("read failed after %v; the read timeout is 300 ms", d)
	}
	c.Close()
}

// The socket options message carries the host's receive and send buffer
// sizes at offsets 0 and 4, at most 1 MiB each, as the reference sends them
// (zeros without VMware's ServerRcvBufSize and ServerSndBufSize; D-0049).
func TestSocketOptions(t *testing.T) {
	for _, c := range []struct {
		rcv, snd uint32
		want     [3]uint32
	}{
		{0, 0, [3]uint32{0, 0, 0}},
		{786432, 524288, [3]uint32{786432, 524288, 0}},
		{4 << 20, 2 << 20, [3]uint32{1 << 20, 1 << 20, 0}},
	} {
		f := newFakeHost(t)
		conn, h, _ := openRead(t, f, Options{ServerRcvBuf: c.rcv, ServerSndBuf: c.snd, SndBuf: 262144, RcvBuf: 393216})
		var got [3]uint32
		for i := range got {
			got[i] = binary.LittleEndian.Uint32(f.gotSockOpts[4*i:])
		}
		if got != c.want {
			t.Errorf("server buffers %d/%d: payload %v, want %v", c.rcv, c.snd, got, c.want)
		}
		conn.CloseDisk(h)
		conn.Close()
	}
}

// A host whose NFC port refuses the connection gives ErrConnect, which Open
// answers with 14009 as the reference does (NBD-0028).
func TestConnectRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // nothing listens there now
	_, err = Dial(context.Background(), "127.0.0.1", Ticket{Host: "127.0.0.1", Port: port}, Options{Timeout: 2 * time.Second})
	if !errors.Is(err, ErrConnect) {
		t.Fatalf("got %v, want ErrConnect", err)
	}
	if ConnectTimeout != 10*time.Second {
		t.Fatalf("connect timeout %v, want 10 s (D-0049)", ConnectTimeout)
	}
}
