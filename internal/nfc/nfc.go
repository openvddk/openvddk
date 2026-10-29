// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package nfc is the subset of the ESXi host's NFC service that the library
// uses: the allocation map of a disk (VixDiskLib_QueryAllocatedBlocks, the
// way VMware's library asks it, NET-0002, D-0036), a disk's descriptor keys
// and its data (the NBD and NBDSSL transports, NET-0003, D-0037).
//
// A session starts with a one-time ticket from vCenter (NfcGetVmFiles). The
// client connects to the host's authentication service (port 902), which
// sends one clear-text line and then expects TLS; inside TLS the client
// presents the ticket and asks for the NFC service. For NBD the same TCP
// connection then carries NFC frames in clear text; for NBDSSL the client
// asks for the TLS variant of the service and the NFC frames run inside a
// second TLS session on that connection, pinned to the same host
// certificate. The frame layouts and the message sequence were read from
// our own lab captures (NET-0002, NET-0003); the commands inside TLS follow
// the open-source OpenVixDiskLib's documentation (docs/CLEAN-ROOM.md) and are
// confirmed by every successful session.
//
// After the handshake one goroutine receives every reply and hands it to
// the request with the same operation number (§43); data arrives in pieces,
// in any order, and is placed by the offset each piece carries. Every
// length, count and offset received is checked before it is used (§19), and
// a protocol violation closes the session: no request completes with data
// that was not received in full.
package nfc

import (
	"bufio"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Ticket is vCenter's answer to NfcGetVmFiles (a HostServiceTicket).
type Ticket struct {
	Host          string // the host's address
	Port          int    // the authentication service's port (902)
	SSLThumbprint string // the host certificate's SHA-1 thumbprint
	Service       string // the service to ask for (vpxa-nfc)
	SessionID     string // one-time session token
}

// Classic frames are 264 bytes; the first uint32 is the type.
const (
	classicSize = 264

	msgSessionComplete    = 4
	msgSessionParams      = 33
	msgSessionParamsReply = 36
	msgHandshake          = 43
	msgVersion            = 51
	msgAIOSession         = 52
	msgConnectionData     = 54
	msgFeatures           = 55

	clientVersion = 11 // what the reference sends; the host answers with its own
	minVersion    = 2  // ESXi 6.7 answers 2 (NBD-0005)
	// Connection data, features and the resource pool need version 3 or
	// later; to a host that answers 2 the reference sends none of them.
	modernVersion = 3
	features      = 3
)

// AIO frames have a 16-byte header: magic, type, payload size, operation
// number. Raw data (a path, a bitmap) follows a payload without a header.
const (
	aioMagic      = 0xA100DA7A
	aioHeaderSize = 16

	aioError        = 1
	aioOpenSession  = 2
	aioCloseSession = 3
	aioOpenFile     = 4
	aioCloseFile    = 5
	aioIO           = 7
	aioSockOpts     = 9
	aioDDBGet       = 11
	aioAllocation   = 13
	aioResPool      = 22

	fileTypeDisk      = 2
	openFlagsReadOnly = 0x800e // the reference's flags for this open (NET-0002)

	ioPayloadSize = 44 // read request and reply piece (NET-0003)
	// ioSkipZeros in a read request (u32 at offset 12) asks the host to leave
	// zero data out of the reply pieces; a piece encoded so carries it too
	// (NET-0004). A piece with code 0 is sent whole.
	ioSkipZeros    = 3
	ddbPayloadSize = 16 // descriptor key request and reply (NET-0003)

	maxPayload   = 4096       // largest fixed payload expected in a reply
	maxChunks    = 512 * 1024 // VixDiskLib's limit per query
	maxPathLen   = 4096
	maxLineSize  = 1024
	maxKeyLen    = 1024
	maxDDBValue  = 64 * 1024
	maxIORequest = 16 << 20 // bytes in one read request
	sectorSize   = 512

	// IOChunk is the size of the read requests Read sends, as the
	// reference sends them (NET-0003).
	IOChunk = 1 << 20
	// DefaultInFlight bounds the requests in flight on one session (§45).
	DefaultInFlight = 8
)

// Errors returned by the package wrap one of these.
var (
	ErrProtocol = errors.New("nfc: unexpected reply from the host")
	ErrRefused  = errors.New("nfc: the host refused the request")
	ErrTLS      = errors.New("nfc: host certificate does not match the ticket")
	ErrClosed   = errors.New("nfc: session closed")
	// ErrConnect: no TCP connection to the host's NFC port within
	// ConnectTimeout, or refused (NBD-0028).
	ErrConnect = errors.New("nfc: no connection to the host")
	// ErrStreamTLS: the host refused TLS for the NFC data stream after the
	// authentication service's TLS worked (NBDSSL not available for this
	// session; a certificate mismatch is ErrTLS instead).
	ErrStreamTLS = errors.New("nfc: TLS for the NFC stream failed")
	// ErrCipherRefused: on a CPU without AES instructions the library
	// offered AES-128-GCM alone over TLS 1.2, and the host refused it for the
	// NFC data stream. The ticket has been used; open again with a new
	// ticket and Options.AllCiphers.
	ErrCipherRefused = errors.New("nfc: the host refused AES-128-GCM alone for the NFC stream")
	// errAuthCiphers: the same refusal by the authentication service, before
	// the ticket was presented; Dial then offers the full list itself.
	errAuthCiphers = errors.New("nfc: the authentication service refused AES-128-GCM alone")
)

// Conn is one NFC session.
type Conn struct {
	c       net.Conn
	op      uint32
	timeout time.Duration
	// replyTimeout is the longest wait for a reply once the session runs:
	// VMware's vixDiskLib.nfc.ReadTimeoutMs, or timeout (D-0049).
	replyTimeout time.Duration
	Version      uint32 // the host's NFC version
	SSL          bool   // the NFC stream runs inside TLS (NBDSSL)
	// TLS is the NFC stream's TLS version and cipher suite with SSL, the
	// authentication service's without ("TLS 1.3, TLS_AES_128_GCM_SHA256").
	TLS string
	// SkipZeros: reads ask the host to leave zero data out (NET-0004).
	SkipZeros bool
	zbuf      []byte // an encoded piece, before it is decoded into the caller's buffer

	engine // requests in flight after the handshake (aio.go)
}

// Options for Dial.
type Options struct {
	Client    string        // client name sent in the connection data (≤ 64 bytes)
	Operation string        // operation name sent in the connection data (≤ 64 bytes)
	Timeout   time.Duration // per step, and the longest wait for a reply; default 60 s
	SSL       bool          // NBDSSL: ask for the TLS service and run NFC inside TLS
	InFlight  int           // requests in flight at most; default DefaultInFlight
	// AllCiphers offers Go's full TLS 1.2 list also on a CPU without AES
	// instructions (after ErrCipherRefused).
	AllCiphers bool
	// SkipZeros asks the host, in every read, to leave zero data out of the
	// reply (the consumer opened with VIXDISKLIB_FLAG_OPEN_COMPRESSION_SKIPZ,
	// NET-0004).
	SkipZeros bool
	// AIOBufSize and AIOBufCount are sent when the AIO session opens: the
	// size in bytes of the host's buffers for this session, which is the
	// largest piece it sends a read in, and their number. 0 leaves the
	// host's default, as the reference does without a setting; VMware's
	// vixDiskLib.nfcAio.Session.BufSizeIn64KB and .BufCount set them
	// (D-0048).
	AIOBufSize  uint32
	AIOBufCount uint32
	// ReadTimeout is the longest wait for a reply once the session runs
	// (VMware's vixDiskLib.nfc.ReadTimeoutMs; 0: Timeout). The host is
	// treated as gone when nothing arrives for that long (D-0049).
	ReadTimeout time.Duration
	// SndBuf and RcvBuf set this side's socket buffers in bytes, and
	// ServerSndBuf and ServerRcvBuf ask the host for its own, in the socket
	// options message, at most MaxServerSockBuf each, as the reference does
	// (VMware's vixDiskLib.nfcAio.SocketOption keys; 0: the defaults; D-0049).
	SndBuf, RcvBuf             int
	ServerSndBuf, ServerRcvBuf uint32
}

// ConnectTimeout is how long a TCP connection to the host's NFC port may
// take: the reference, too, fails Open after this time when the host does not answer,
// whatever vixDiskLib.nfc.AcceptTimeoutMs says (NBD-0028).
const ConnectTimeout = 10 * time.Second

// MaxServerSockBuf is the largest host socket buffer the reference asks for:
// larger settings are sent as 1 MiB (observed, D-0049).
const MaxServerSockBuf = 1 << 20

// Dial opens an NFC session to addr (the ticket's host, or the host's
// backup-NFC address) with the ticket. On a CPU without AES instructions it
// offers AES-128-GCM alone over TLS 1.2 (aes128Suites), unless
// o.AllCiphers; when the authentication service refuses that, it connects
// again with the full list (the ticket is not presented before that TLS).
func Dial(ctx context.Context, addr string, t Ticket, o Options) (*Conn, error) {
	aes128 := !aesHardware && !o.AllCiphers
	c, err := dial(ctx, addr, t, o, aes128)
	if aes128 && errors.Is(err, errAuthCiphers) {
		c, err = dial(ctx, addr, t, o, false)
	}
	return c, err
}

func dial(ctx context.Context, addr string, t Ticket, o Options, aes128 bool) (*Conn, error) {
	if o.Timeout == 0 {
		o.Timeout = 60 * time.Second
	}
	if len(o.Client) > 64 || len(o.Operation) > 64 {
		return nil, fmt.Errorf("nfc: client or operation name too long")
	}
	port := t.Port
	if port == 0 {
		port = 902
	}
	d := net.Dialer{Timeout: ConnectTimeout}
	raw, err := d.DialContext(ctx, "tcp", net.JoinHostPort(addr, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("nfc: connect %s:%d: %w: %w", addr, port, ErrConnect, err)
	}
	if tcp, ok := raw.(*net.TCPConn); ok {
		// Before TLS, as the reference sets them on its socket (D-0049).
		if o.SndBuf > 0 {
			_ = tcp.SetWriteBuffer(o.SndBuf)
		}
		if o.RcvBuf > 0 {
			_ = tcp.SetReadBuffer(o.RcvBuf)
		}
	}
	reply := o.ReadTimeout
	if reply <= 0 {
		reply = o.Timeout
	}
	c := &Conn{c: raw, timeout: o.Timeout, replyTimeout: reply, SSL: o.SSL, SkipZeros: o.SkipZeros}
	if err := c.authenticate(addr, t, o.SSL, aes128); err != nil {
		raw.Close()
		return nil, err
	}
	if err := c.handshake(o); err != nil {
		c.c.Close()
		return nil, err
	}
	n := o.InFlight
	if n <= 0 {
		n = DefaultInFlight
	}
	c.start(n)
	return c, nil
}

func (c *Conn) deadline() { _ = c.c.SetDeadline(time.Now().Add(c.timeout)) }

// authenticate reads the clear-text greeting, wraps the connection in TLS
// (pinning the ticket's thumbprint), presents the ticket and asks for the
// NFC service. NFC then continues on the raw connection, or, with ssl, inside
// a second TLS session on it, pinned to the same certificate. With aes128
// both offer AES-128-GCM alone over TLS 1.2; a TLS-level refusal of that is
// errAuthCiphers for the first session, ErrCipherRefused for the second.
func (c *Conn) authenticate(addr string, t Ticket, ssl, aes128 bool) error {
	c.deadline()
	line, err := readLineRaw(c.c)
	if err != nil {
		return fmt.Errorf("nfc: authentication service greeting: %w", err)
	}
	if !strings.HasPrefix(line, "220 ") {
		return fmt.Errorf("%w: greeting %.40q", ErrProtocol, line)
	}
	cfg, err := pinnedTLS(addr, t.SSLThumbprint, aes128)
	if err != nil {
		return err
	}
	tc := tls.Client(c.c, cfg)
	if err := tc.Handshake(); err != nil {
		if aes128 && !errors.Is(err, ErrTLS) && tlsRefusal(err) {
			return fmt.Errorf("%w: %v", errAuthCiphers, err)
		}
		return fmt.Errorf("nfc: TLS with the host: %w", err)
	}
	c.TLS = tlsState(tc.ConnectionState())
	br := bufio.NewReaderSize(tc, 512)
	send := func(s string) error {
		_, err := io.WriteString(tc, s+"\r\n")
		return err
	}
	expect := func(prefix, what string) (string, error) {
		l, err := readLine(br)
		if err != nil {
			return "", fmt.Errorf("nfc: %s: %w", what, err)
		}
		if !strings.HasPrefix(l, prefix) {
			return "", fmt.Errorf("%w: %s: %.60q", ErrRefused, what, l)
		}
		return l, nil
	}
	// vCenter 7.0 and 8.0 give the session ID as a plain UUID; vCenter 6.7
	// gives it in VMware's spaced form ("52 1a … 70-81 92 …", 47
	// characters), which is sent as it is. A line break would start another
	// command, so it is refused.
	if strings.ContainsAny(t.SessionID, "\r\n") || strings.TrimSpace(t.SessionID) == "" || strings.ContainsAny(t.Service, "\r\n ") {
		return fmt.Errorf("%w: malformed ticket", ErrProtocol)
	}
	// The session token gets no reply of its own.
	if err := send("SESSION " + t.SessionID); err != nil {
		return err
	}
	if err := send("BANNER "); err != nil {
		return err
	}
	if _, err := expect("220", "banner"); err != nil {
		return err
	}
	// A host whose greeting says "SHA256 supported" (ESXi 7.0 and later) is
	// asked for its SHA-256 thumbprint, ESXi 6.7 for its SHA-1 one, as the
	// reference asks them (NBD-0005). The host certificate is pinned by TLS
	// above; the reply is not used.
	thumbCmd := "THUMBPRINT PlainText"
	if strings.Contains(line, "SHA256 supported") {
		thumbCmd = "THUMBPRINT_SHA2 PlainText"
	}
	if err := send(thumbCmd); err != nil {
		return err
	}
	if _, err := expect("200", "thumbprint"); err != nil {
		return err
	}
	service := t.Service
	if service == "" {
		service = "vpxa-nfc"
	}
	if ssl && !strings.HasSuffix(service, "ssl") {
		service += "ssl"
	}
	if err := send("PROXY " + service); err != nil {
		return err
	}
	if _, err := expect("200", "service "+service); err != nil {
		return err
	}
	if br.Buffered() != 0 {
		return fmt.Errorf("%w: data after the service reply", ErrProtocol)
	}
	if ssl {
		nc := tls.Client(c.c, cfg)
		if err := nc.Handshake(); err != nil {
			if errors.Is(err, ErrTLS) || !tlsRefusal(err) {
				return fmt.Errorf("nfc: TLS for the NFC stream: %w", err)
			}
			if aes128 {
				// Never ErrStreamTLS: a refusal caused by the shortened offer
				// must not let an NBDSSL open go on to clear-text NBD.
				return fmt.Errorf("%w: %v", ErrCipherRefused, err)
			}
			return fmt.Errorf("%w: %v", ErrStreamTLS, err)
		}
		c.TLS = tlsState(nc.ConnectionState())
		c.c = nc
	}
	return nil
}

// tlsRefusal reports whether a failed TLS handshake was the peer's answer
// at the TLS level: a TLS alert, or a reply that is not TLS at all. A
// reset, an end of stream or a timeout is not: anyone on the path can
// cause those, so they never count as the host refusing TLS.
func tlsRefusal(err error) bool {
	var rh tls.RecordHeaderError
	if errors.As(err, &rh) {
		return true
	}
	var oe *net.OpError
	return errors.As(err, &oe) && oe.Op == "remote error"
}

// handshake runs the classic messages up to the AIO session.
func (c *Conn) handshake(o Options) error {
	name, opName := o.Client, o.Operation
	if name == "" {
		name = "openvddk"
	}
	if opName == "" {
		opName = "allocation"
	}
	c.deadline()
	hs := classic(msgHandshake)
	copy(hs[4:], "PlainText")
	if err := c.write(hs); err != nil {
		return err
	}
	if err := c.write(classic(msgSessionParams)); err != nil {
		return err
	}
	if _, err := c.readClassic(msgSessionParamsReply); err != nil {
		return err
	}
	v := classic(msgVersion)
	binary.LittleEndian.PutUint32(v[4:], clientVersion)
	if err := c.write(v); err != nil {
		return err
	}
	r, err := c.readClassic(msgVersion)
	if err != nil {
		return err
	}
	c.Version = binary.LittleEndian.Uint32(r[4:])
	if c.Version < minVersion {
		return fmt.Errorf("%w: host NFC version %d", ErrProtocol, c.Version)
	}
	modern := c.Version >= modernVersion
	msgs := [][]byte{classic(msgAIOSession)}
	if modern {
		cd := classic(msgConnectionData)
		binary.LittleEndian.PutUint32(cd[4:], uint32(len(name)))
		binary.LittleEndian.PutUint32(cd[8:], uint32(len(opName)))
		f := classic(msgFeatures)
		binary.LittleEndian.PutUint32(f[4:], features)
		msgs = [][]byte{cd, []byte(name), []byte(opName), f, classic(msgAIOSession)}
	}
	// One write per message, as the reference sends them: inside TLS each
	// becomes its own record, and the host waits for the next message on
	// the socket (NBDSSL stalled when several shared one record).
	for _, m := range msgs {
		if err := c.write(m); err != nil {
			return err
		}
	}
	if _, err := c.readClassic(msgAIOSession); err != nil {
		return err
	}
	session := make([]byte, 16)
	binary.LittleEndian.PutUint32(session[4:], o.AIOBufSize)
	binary.LittleEndian.PutUint32(session[8:], o.AIOBufCount)
	if _, _, err := c.call(aioOpenSession, session, nil, 16); err != nil {
		return err
	}
	sock := make([]byte, 12)
	binary.LittleEndian.PutUint32(sock[0:], min(o.ServerRcvBuf, MaxServerSockBuf))
	binary.LittleEndian.PutUint32(sock[4:], min(o.ServerSndBuf, MaxServerSockBuf))
	if _, _, err := c.call(aioSockOpts, sock, nil, 12); err != nil {
		return err
	}
	if !modern {
		return nil
	}
	pool := make([]byte, 4)
	binary.LittleEndian.PutUint32(pool, 1)
	_, _, err = c.call(aioResPool, pool, nil, 4)
	return err
}

// ParseOpenReply returns the handle from an OPEN_FILE reply payload.
func ParseOpenReply(r []byte) (uint64, error) {
	if len(r) < 24 {
		return 0, fmt.Errorf("%w: open reply of %d bytes", ErrProtocol, len(r))
	}
	if t := binary.LittleEndian.Uint32(r[16:]); t != fileTypeDisk {
		return 0, fmt.Errorf("%w: opened file of type %d", ErrProtocol, t)
	}
	return binary.LittleEndian.Uint64(r[8:]), nil
}

// ParseAllocationReply checks an allocation reply payload against the
// request's chunk count and returns the bitmap's length in 32-bit words.
func ParseAllocationReply(r []byte, n uint64) (uint64, error) {
	if len(r) < 48 {
		return 0, fmt.Errorf("%w: allocation reply of %d bytes", ErrProtocol, len(r))
	}
	if echo := binary.LittleEndian.Uint64(r[32:]); echo != n {
		return 0, fmt.Errorf("%w: allocation reply for %d chunks, asked %d", ErrProtocol, echo, n)
	}
	words := uint64(binary.LittleEndian.Uint32(r[44:]))
	if words != (n+31)/32 {
		return 0, fmt.Errorf("%w: bitmap of %d words for %d chunks", ErrProtocol, words, n)
	}
	return words, nil
}

// ParseIOPiece checks one reply piece of a read request (NET-0003) against
// the request (byte offset off, length n) and returns the piece's offset
// within the request and its length. A piece must lie inside the request
// and on whole sectors.
func ParseIOPiece(r []byte, off uint64, n uint32) (pieceOff, pieceLen uint32, err error) {
	if len(r) < ioPayloadSize {
		return 0, 0, fmt.Errorf("%w: read reply of %d bytes", ErrProtocol, len(r))
	}
	if o := binary.LittleEndian.Uint64(r[16:]); o != off {
		return 0, 0, fmt.Errorf("%w: read reply for offset %d, asked %d", ErrProtocol, o, off)
	}
	if l := binary.LittleEndian.Uint32(r[24:]); l != n {
		return 0, 0, fmt.Errorf("%w: read reply for %d bytes, asked %d", ErrProtocol, l, n)
	}
	pieceOff = binary.LittleEndian.Uint32(r[28:])
	pieceLen = binary.LittleEndian.Uint32(r[32:])
	if pieceLen == 0 || pieceOff%sectorSize != 0 || pieceLen%sectorSize != 0 ||
		uint64(pieceOff)+uint64(pieceLen) > uint64(n) {
		return 0, 0, fmt.Errorf("%w: read piece %d+%d of a %d-byte request", ErrProtocol, pieceOff, pieceLen, n)
	}
	return pieceOff, pieceLen, nil
}

// ParseIOEncoding reads how a read reply piece of pieceLen bytes is sent:
// whole (code 0: pieceLen bytes follow, whatever the second length says, as
// before skipz) or with its zeros left out (code ioSkipZeros: the second
// length gives the bytes that follow; only when asked, NET-0004).
func ParseIOEncoding(r []byte, pieceLen uint32, asked bool) (wire uint32, skipZeros bool, err error) {
	if len(r) < ioPayloadSize {
		return 0, false, fmt.Errorf("%w: read reply of %d bytes", ErrProtocol, len(r))
	}
	code, wire := binary.LittleEndian.Uint32(r[12:]), binary.LittleEndian.Uint32(r[36:])
	switch {
	case code == 0:
		return pieceLen, false, nil
	case code == ioSkipZeros && asked && wire >= 8 && wire <= 2*pieceLen+8:
		return wire, true, nil
	}
	return 0, false, fmt.Errorf("%w: read piece of %d bytes sent as %d bytes with code %d", ErrProtocol, pieceLen, wire, code)
}

// decodeSkipZeros rebuilds a piece sent with its zeros left out (NET-0004):
// the piece's length and a zero word, then runs of (offset in the piece,
// length, that many bytes) in ascending order; the rest of the piece is zero.
func decodeSkipZeros(dst, src []byte) error {
	bad := func(what string) error {
		return fmt.Errorf("%w: zero-skipped read piece: %s", ErrProtocol, what)
	}
	if len(src) < 8 {
		return bad("shorter than its header")
	}
	if n := binary.LittleEndian.Uint32(src); uint64(n) != uint64(len(dst)) || binary.LittleEndian.Uint32(src[4:]) != 0 {
		return bad(fmt.Sprintf("header %x for a %d-byte piece", src[:8], len(dst)))
	}
	clear(dst)
	next := uint64(0)
	for pos := 8; pos < len(src); {
		if len(src)-pos < 8 {
			return bad("run header cut short")
		}
		off, n := uint64(binary.LittleEndian.Uint32(src[pos:])), uint64(binary.LittleEndian.Uint32(src[pos+4:]))
		pos += 8
		if n == 0 || off < next || off+n > uint64(len(dst)) || n > uint64(len(src)-pos) {
			return bad(fmt.Sprintf("run %d+%d", off, n))
		}
		copy(dst[off:off+n], src[pos:])
		pos += int(n)
		next = off + n
	}
	return nil
}

// ParseDDBReply returns the length of the value that follows a descriptor
// key reply (0: the key is not set).
func ParseDDBReply(r []byte) (uint32, error) {
	if len(r) < ddbPayloadSize {
		return 0, fmt.Errorf("%w: descriptor key reply of %d bytes", ErrProtocol, len(r))
	}
	n := binary.LittleEndian.Uint32(r[12:])
	if n > maxDDBValue {
		return 0, fmt.Errorf("%w: descriptor value of %d bytes", ErrProtocol, n)
	}
	return n, nil
}

// call sends an AIO request and reads its reply payload (at most want
// bytes are expected; a longer payload is refused).
func (c *Conn) call(typ uint32, payload, extra []byte, want int) ([]byte, uint32, error) {
	c.deadline()
	if err := c.send(typ, payload, extra); err != nil {
		return nil, 0, err
	}
	r, err := c.reply(typ, want)
	return r, c.op - 1, err
}

func (c *Conn) send(typ uint32, payload, extra []byte) error {
	h := make([]byte, aioHeaderSize, aioHeaderSize+len(payload)+len(extra))
	binary.LittleEndian.PutUint32(h[0:], aioMagic)
	binary.LittleEndian.PutUint32(h[4:], typ)
	binary.LittleEndian.PutUint32(h[8:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(h[12:], c.op)
	c.op++
	if err := c.write(append(h, payload...)); err != nil {
		return err
	}
	if len(extra) > 0 {
		return c.write(extra) // a separate write, as the reference sends it
	}
	return nil
}

// reply reads one AIO reply for the last request of type typ.
func (c *Conn) reply(typ uint32, want int) ([]byte, error) {
	h := make([]byte, aioHeaderSize)
	if _, err := io.ReadFull(c.c, h); err != nil {
		return nil, fmt.Errorf("nfc: reply header: %w", err)
	}
	gotType, size, op, err := ParseAIOHeader(h)
	if err != nil {
		return nil, err
	}
	if size > maxPayload {
		return nil, fmt.Errorf("%w: reply payload of %d bytes", ErrProtocol, size)
	}
	p := make([]byte, size)
	if _, err := io.ReadFull(c.c, p); err != nil {
		return nil, fmt.Errorf("nfc: reply payload: %w", err)
	}
	if gotType == aioError {
		return nil, fmt.Errorf("%w: request type %d: error reply %s", ErrRefused, typ, hex.EncodeToString(p[:min(len(p), 32)]))
	}
	if gotType != typ || op != c.op-1 {
		return nil, fmt.Errorf("%w: reply type %d op %d to request type %d op %d", ErrProtocol, gotType, op, typ, c.op-1)
	}
	if int(size) > want {
		return nil, fmt.Errorf("%w: reply type %d of %d bytes, expected %d", ErrProtocol, typ, size, want)
	}
	return p, nil
}

// ParseAIOHeader validates an AIO header and returns type, payload size
// and operation number.
func ParseAIOHeader(h []byte) (typ, size, op uint32, err error) {
	if len(h) < aioHeaderSize {
		return 0, 0, 0, fmt.Errorf("%w: short header", ErrProtocol)
	}
	if m := binary.LittleEndian.Uint32(h[0:]); m != aioMagic {
		return 0, 0, 0, fmt.Errorf("%w: bad frame magic %#x", ErrProtocol, m)
	}
	return binary.LittleEndian.Uint32(h[4:]), binary.LittleEndian.Uint32(h[8:]), binary.LittleEndian.Uint32(h[12:]), nil
}

func (c *Conn) readClassic(want uint32) ([]byte, error) {
	b := make([]byte, classicSize)
	if _, err := io.ReadFull(c.c, b); err != nil {
		return nil, fmt.Errorf("nfc: reply to message %d: %w", want, err)
	}
	if t := binary.LittleEndian.Uint32(b); t != want {
		return nil, fmt.Errorf("%w: message %d where %d was expected", ErrProtocol, t, want)
	}
	return b, nil
}

func (c *Conn) write(b []byte) error {
	if _, err := c.c.Write(b); err != nil {
		return fmt.Errorf("nfc: send: %w", err)
	}
	return nil
}

func classic(typ uint32) []byte {
	b := make([]byte, classicSize)
	binary.LittleEndian.PutUint32(b, typ)
	return b
}

// readLineRaw reads one CRLF- or LF-terminated line byte by byte, so that
// nothing after it is consumed (the TLS handshake follows).
func readLineRaw(r io.Reader) (string, error) {
	var b []byte
	one := make([]byte, 1)
	for len(b) < maxLineSize {
		if _, err := io.ReadFull(r, one); err != nil {
			return "", err
		}
		if one[0] == '\n' {
			return strings.TrimRight(string(b), "\r"), nil
		}
		b = append(b, one[0])
	}
	return "", fmt.Errorf("%w: line longer than %d bytes", ErrProtocol, maxLineSize)
}

func readLine(br *bufio.Reader) (string, error) {
	var b []byte
	for len(b) < maxLineSize {
		c, err := br.ReadByte()
		if err != nil {
			return "", err
		}
		if c == '\n' {
			return strings.TrimRight(string(b), "\r"), nil
		}
		b = append(b, c)
	}
	return "", fmt.Errorf("%w: line longer than %d bytes", ErrProtocol, maxLineSize)
}

// tlsState names a session's TLS version and cipher suite.
func tlsState(s tls.ConnectionState) string {
	return tls.VersionName(s.Version) + ", " + tls.CipherSuiteName(s.CipherSuite)
}

// pinnedTLS verifies the host certificate against the ticket's thumbprint
// (SHA-1, or SHA-256 when given) instead of a chain. With aes128 the TLS 1.2
// offer is AES-128-GCM alone (aes128Suites); TLS 1.3 is offered as always.
func pinnedTLS(server, thumbprint string, aes128 bool) (*tls.Config, error) {
	h := strings.NewReplacer(":", "", " ", "", "-", "").Replace(strings.TrimSpace(thumbprint))
	want, err := hex.DecodeString(h)
	if err != nil || (len(want) != sha1.Size && len(want) != sha256.Size) {
		return nil, fmt.Errorf("%w: malformed thumbprint in the ticket", ErrTLS)
	}
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         strings.Trim(server, "[]"),
		InsecureSkipVerify: true, // replaced by the pin below, which is always enforced
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return fmt.Errorf("%w: no certificate", ErrTLS)
			}
			var got []byte
			if len(want) == sha1.Size {
				s := sha1.Sum(raw[0])
				got = s[:]
			} else {
				s := sha256.Sum256(raw[0])
				got = s[:]
			}
			if subtle.ConstantTimeCompare(got, want) != 1 {
				return ErrTLS
			}
			return nil
		},
	}
	if aes128 {
		cfg.CipherSuites = aes128Suites
	}
	return cfg, nil
}

// Runs turns an allocation bitmap of n chunks into runs of set bits, as
// [first, end) chunk indexes.
func Runs(bm []byte, n uint64) [][2]uint64 {
	var out [][2]uint64
	var start uint64
	in := false
	for i := uint64(0); i < n; i++ {
		set := i/8 < uint64(len(bm)) && bm[i/8]&(1<<(i%8)) != 0
		switch {
		case set && !in:
			start, in = i, true
		case !set && in:
			out = append(out, [2]uint64{start, i})
			in = false
		}
	}
	if in {
		out = append(out, [2]uint64{start, n})
	}
	return out
}
