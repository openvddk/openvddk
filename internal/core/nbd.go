// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/openvddk/openvddk/internal/config"
	"github.com/openvddk/openvddk/internal/lifecycle"
	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/nfc"
	"github.com/openvddk/openvddk/internal/readahead"
	"github.com/openvddk/openvddk/internal/steptime"
	"github.com/openvddk/openvddk/internal/transport"
	"github.com/openvddk/openvddk/internal/vix"
	"github.com/openvddk/openvddk/internal/vsphere"
)

// nbdDisk is a disk read over NBD or NBDSSL (D-0037, NET-0003): one NFC
// session to the host that holds the disk, the disk opened read-only by the
// path the caller gave, as the reference opens it, and reads as NFC read
// requests, fetched ahead for sequential readers. With OpenVDDK.nbd.sessions
// above 1 the disk is open in that many sessions and the read requests are
// spread over them (D-0045); the first session alone answers allocation
// and descriptor queries.
type nbdDisk struct {
	conn   *nfc.Conn // the first session
	handle uint64
	more   []nbdSession // the further sessions, for reads only
	reads  *nfc.Spread  // every session
	ra     *readahead.Reader
}

// nbdSession is one further NFC session with the disk open in it.
type nbdSession struct {
	conn   *nfc.Conn
	handle uint64
}

// ReadFull reads len(p) bytes at byte offset off from the host.
func (n *nbdDisk) ReadFull(p []byte, off int64) error {
	return n.reads.Read(uint64(off), p)
}

// start sends one read request of at most nfc.IOChunk bytes, in the next
// session in turn.
func (n *nbdDisk) start(off uint64, p []byte) (*nfc.Op, error) {
	return n.reads.ReadStart(off, p)
}

// read serves a consumer read, through the read-ahead when it runs.
func (n *nbdDisk) read(p []byte, off int64) error {
	if n.ra != nil {
		return n.ra.ReadFull(p, off)
	}
	return n.ReadFull(p, off)
}

func (n *nbdDisk) close(path string) error {
	if n.ra != nil {
		n.ra.Close() // waits for the reads in flight
	}
	err := closeNBDSession(path, n.conn, n.handle)
	for _, s := range n.more {
		if cerr := closeNBDSession(path, s.conn, s.handle); err == nil {
			err = cerr
		}
	}
	return err
}

// closeNBDSession closes the disk in one session and ends the session.
func closeNBDSession(path string, c *nfc.Conn, h uint64) error {
	if ferr := c.Err(); ferr != nil {
		// The session ended earlier (a read reported it): the host released
		// the file when the connection closed, so nothing is left to close.
		c.Close()
		logx.Close.NFC().Infof("%s: the NFC session had already ended (%v); nothing to release on the host", path, ferr)
		return nil
	}
	err := c.CloseDisk(h)
	if cerr := c.Close(); err == nil {
		err = cerr
	}
	return err
}

// nbdSettings are the NBD and NBDSSL settings of the configuration file.
type nbdSettings struct {
	inFlight int              // requests in flight per session
	sessions int              // NFC sessions per open disk
	ra       readahead.Config // the read-ahead, when raOn
	raOn     bool
	aioBuf   uint32 // NFC AIO buffer size in bytes asked of the host (0: its default)
	aioCount uint32 // NFC AIO buffers asked of the host (0: its default)
	// VMware's vixDiskLib.nfc.ReadTimeoutMs and the four
	// vixDiskLib.nfcAio.SocketOption keys (D-0049); 0: not set.
	readTimeout                time.Duration
	sndBuf, rcvBuf             int
	serverSndBuf, serverRcvBuf uint32
}

// nbdDefaults: one NFC session per disk, keeping up to 16 requests of 1 MiB
// in flight; the read-ahead fetches 2 MiB blocks, 8 ahead, after two
// sequential reads.
var nbdDefaults = nbdSettings{inFlight: 16, sessions: 1, ra: readahead.Config{Block: 2 << 20, Depth: 8, Align: 512, Trigger: 2}, raOn: true}

// nbdConfig reads OpenVDDK.nbd.inFlight (1 to 64), OpenVDDK.nbd.sessions (1 to
// 16), OpenVDDK.nbd.readAheadKB (a multiple of 512 bytes up to 65536 KiB; 0
// disables read-ahead) and OpenVDDK.nbd.readAheadDepth (1 to 64), and
// VMware's documented vixDiskLib.nfcAio.Session.BufSizeIn64KB (1 to 32) and
// vixDiskLib.nfcAio.Session.BufCount (1 to 16), which the host is asked for
// when the session opens (D-0048); invalid values keep the default and are
// logged.
func nbdConfig(cfg *config.Config) nbdSettings {
	st := nbdDefaults
	if v, ok := cfg.Get("OpenVDDK.nbd.inFlight"); ok {
		if x, err := strconv.Atoi(v); err == nil && x >= 1 && x <= 64 {
			st.inFlight = x
		} else {
			logx.OpenEx.NFC().Warnf("OpenVDDK.nbd.inFlight = %q is not 1 to 64; using %d", v, st.inFlight)
		}
	}
	if v, ok := cfg.Get("OpenVDDK.nbd.sessions"); ok {
		if x, err := strconv.Atoi(v); err == nil && x >= 1 && x <= 16 {
			st.sessions = x
		} else {
			logx.OpenEx.NFC().Warnf("OpenVDDK.nbd.sessions = %q is not 1 to 16; using %d", v, st.sessions)
		}
	}
	if v, ok := cfg.Get("OpenVDDK.nbd.readAheadKB"); ok {
		x, err := strconv.Atoi(v)
		switch {
		case err == nil && x == 0:
			st.raOn = false
		case err == nil && x > 0 && x <= 65536 && (x*1024)%512 == 0:
			st.ra.Block = x * 1024
		default:
			logx.OpenEx.NFC().Warnf("OpenVDDK.nbd.readAheadKB = %q is not valid; using %d", v, st.ra.Block/1024)
		}
	}
	if v, ok := cfg.Get("OpenVDDK.nbd.readAheadDepth"); ok {
		if x, err := strconv.Atoi(v); err == nil && x >= 1 && x <= 64 {
			st.ra.Depth = x
		} else {
			logx.OpenEx.NFC().Warnf("OpenVDDK.nbd.readAheadDepth = %q is not 1 to 64; using %d", v, st.ra.Depth)
		}
	}
	if v, ok := cfg.Get("vixDiskLib.nfcAio.Session.BufSizeIn64KB"); ok {
		if x, err := strconv.Atoi(v); err == nil && x >= 1 && x <= 32 {
			st.aioBuf = uint32(x) << 16
		} else {
			logx.OpenEx.NFC().Warnf("vixDiskLib.nfcAio.Session.BufSizeIn64KB = %q is not 1 to 32; the host's default is used", v)
		}
	}
	if v, ok := cfg.Get("vixDiskLib.nfc.ReadTimeoutMs"); ok {
		if x, err := strconv.Atoi(v); err == nil && x >= 1 && x <= math.MaxInt32 {
			st.readTimeout = time.Duration(x) * time.Millisecond
		} else {
			logx.OpenEx.NFC().Warnf("vixDiskLib.nfc.ReadTimeoutMs = %q is not a positive number of milliseconds; the default is used", v)
		}
	}
	for _, k := range []struct {
		key string
		set func(int)
	}{
		{"vixDiskLib.nfcAio.SocketOption.SndBufSize", func(x int) { st.sndBuf = x }},
		{"vixDiskLib.nfcAio.SocketOption.RcvBufSize", func(x int) { st.rcvBuf = x }},
		{"vixDiskLib.nfcAio.SocketOption.ServerSndBufSize", func(x int) { st.serverSndBuf = uint32(x) }},
		{"vixDiskLib.nfcAio.SocketOption.ServerRcvBufSize", func(x int) { st.serverRcvBuf = uint32(x) }},
	} {
		if v, ok := cfg.Get(k.key); ok {
			if x, err := strconv.Atoi(v); err == nil && x >= 1 && x <= math.MaxInt32 {
				k.set(x)
			} else {
				logx.OpenEx.NFC().Warnf("%s = %q is not a positive number of bytes; the default is used", k.key, v)
			}
		}
	}
	if v, ok := cfg.Get("vixDiskLib.nfcAio.Session.BufCount"); ok {
		if x, err := strconv.Atoi(v); err == nil && x >= 1 && x <= 16 {
			st.aioCount = uint32(x)
		} else {
			logx.OpenEx.NFC().Warnf("vixDiskLib.nfcAio.Session.BufCount = %q is not 1 to 16; the host's default is used", v)
		}
	}
	return st
}

// openNBD opens the disk over NBD (mode nbd) or NBDSSL (mode nbdssl): a
// one-time ticket for the VM from vCenter, or from the ESXi host itself on a
// direct connection (D-0052), an NFC session to the host (its
// vSphere Backup NFC address when one is set), the disk opened by path.
func openNBD(ctx context.Context, c *Connection, sess *vsphere.Session, det vsphere.DiskDetails, path string, mode transport.Mode, flags uint32, t *steptime.Timer) (*Disk, error) {
	info, err := infoFrom(ctx, sess, det)
	if err != nil {
		return nil, err
	}
	t.Mark("descriptors")
	tk, err := sess.NfcTicket(ctx, det.VM)
	if err != nil {
		return nil, err
	}
	addr := sess.NfcAddress(ctx, tk, det.VM)
	t.Mark("ticket")
	st := nbdConfig(lifecycle.Global.Config())
	// Of the compression flags only skipz changes the reads, as the reference
	// does it (NET-0004); zlib and fastlz are accepted and read whole.
	opts := nfc.Options{SSL: mode == transport.NBDSSL, Operation: "nbdmode", InFlight: st.inFlight, SkipZeros: flags&FlagSkipZeros != 0,
		AIOBufSize: st.aioBuf, AIOBufCount: st.aioCount, ReadTimeout: st.readTimeout,
		SndBuf: st.sndBuf, RcvBuf: st.rcvBuf, ServerSndBuf: st.serverSndBuf, ServerRcvBuf: st.serverRcvBuf}
	if opts.SSL && !nfc.AESHardware() {
		logx.OpenEx.NFC().Warnf("%s: this CPU gives the library no AES instructions, so NBDSSL decrypts in software and reads several times more slowly; give this virtual machine the host's CPU features (for example libvirt's host-passthrough or host-model CPU mode)", path)
	}
	nc, err := nfc.Dial(ctx, addr, tk, opts)
	if errors.Is(err, nfc.ErrCipherRefused) {
		// The host refused the AES-128-GCM offer for the data stream after
		// the ticket was presented: once more with a new ticket and the full
		// cipher list, never on to clear-text NBD for this reason.
		logx.OpenEx.NFC().Warnf("%s: %s refused AES-128-GCM alone for the NFC stream; connecting again with the full cipher list", path, addr)
		if tk, err = sess.NfcTicket(ctx, det.VM); err == nil {
			addr = sess.NfcAddress(ctx, tk, det.VM)
			opts.AllCiphers = true
			nc, err = nfc.Dial(ctx, addr, tk, opts)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("NFC session with %s: %w", addr, err)
	}
	if opts.SSL {
		logx.OpenEx.NFC().Infof("%s: NFC stream encrypted with %s", path, nc.TLS)
	}
	if opts.SkipZeros {
		logx.OpenEx.NFC().Infof("%s: reads ask the host to leave zero data out (skipz)", path)
	}
	if opts.ReadTimeout != 0 || opts.SndBuf != 0 || opts.RcvBuf != 0 || opts.ServerSndBuf != 0 || opts.ServerRcvBuf != 0 {
		logx.OpenEx.NFC().Infof("%s: NFC read timeout %v, socket buffers %d/%d bytes, the host's asked %d/%d bytes (0: the default; the host's at most %d)",
			path, opts.ReadTimeout, opts.SndBuf, opts.RcvBuf, opts.ServerSndBuf, opts.ServerRcvBuf, nfc.MaxServerSockBuf)
	}
	if opts.AIOBufSize != 0 || opts.AIOBufCount != 0 {
		logx.OpenEx.NFC().Infof("%s: NFC AIO buffers asked of the host: size %d KiB, count %d (0: the host's default)", path, opts.AIOBufSize>>10, opts.AIOBufCount)
	}
	t.Mark("session")
	h, err := nc.OpenDisk(path)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("open over NFC: %w", err)
	}
	t.Mark("open")
	nd := &nbdDisk{conn: nc, handle: h}
	conns, handles := []*nfc.Conn{nc}, []uint64{h}
	for i := 1; i < st.sessions; i++ {
		s, err := openNBDSession(ctx, sess, det.VM, path, opts)
		if err != nil {
			// The reads work in the sessions already open; fewer sessions
			// only read more slowly from a slow datastore.
			logx.OpenEx.NFC().Warnf("%s: NFC session %d of %d not opened (%v); reading in %d", path, i+1, st.sessions, err, i)
			break
		}
		nd.more = append(nd.more, s)
		conns, handles = append(conns, s.conn), append(handles, s.handle)
	}
	if len(conns) > 1 {
		t.Mark("sessions")
	}
	if nd.reads, err = nfc.NewSpread(conns, handles); err != nil {
		nd.close(path)
		return nil, err
	}
	if st.raOn {
		nd.ra = readahead.New(nd, det.CapacityBytes, st.ra)
	}
	logx.OpenEx.NFC().Infof("%s: opened over %s through %s (NFC version %d)", path, mode, addr, nc.Version)
	if len(conns) > 1 {
		logx.OpenEx.NFC().Infof("%s: reads spread over %d NFC sessions", path, len(conns))
	}
	return &Disk{conn: c, mode: mode, path: path, nbd: nd, sectors: uint64(det.CapacityBytes / 512), info: info, ds: det.Datastore, dpath: det.Path,
		vm: det.VM, snapshot: det.Snapshot, devKey: det.Key, top: det.Top,
		readOnly: true, opened: time.Now(), stats: readStats{nextMark: progressStep}}, nil
}

// openNBDSession opens the disk in one more NFC session: a new one-time
// ticket, a session with the first session's options (so with the cipher
// list it ended up with), the disk opened by path.
func openNBDSession(ctx context.Context, sess *vsphere.Session, vm, path string, opts nfc.Options) (nbdSession, error) {
	tk, err := sess.NfcTicket(ctx, vm)
	if err != nil {
		return nbdSession{}, err
	}
	addr := sess.NfcAddress(ctx, tk, vm)
	nc, err := nfc.Dial(ctx, addr, tk, opts)
	if err != nil {
		return nbdSession{}, fmt.Errorf("NFC session with %s: %w", addr, err)
	}
	h, err := nc.OpenDisk(path)
	if err != nil {
		nc.Close()
		return nbdSession{}, fmt.Errorf("open over NFC: %w", err)
	}
	return nbdSession{conn: nc, handle: h}, nil
}

// nbdOpenCode is Open's answer when NBD or NBDSSL cannot open the disk: a
// missing vCenter privilege and a refused open are 13, as for HotAdd
// (D-0032); a host certificate that does not match the ticket is the SSL
// security error; network failures map to the host codes (vix.ToCode).
// The codes are to be compared with the reference's under fault injection
// (§72).
func nbdOpenCode(err error) vix.Code {
	switch {
	case errors.Is(err, vsphere.ErrNoPermission), errors.Is(err, nfc.ErrRefused):
		return vix.EFileAccessError
	case errors.Is(err, nfc.ErrTLS):
		return vix.ENetHTTPSSLSecurity
	case errors.Is(err, nfc.ErrConnect):
		// No connection to the host's NFC port, silent or refused: 14009,
		// as the reference answers (NBD-0011, NBD-0028).
		return vix.EHostConnRefused
	}
	return vix.ToCode(err)
}

// nbdReadCode is the code of a failed read over NBD or NBDSSL. When the NFC
// connection is lost (reset, no reply within the timeout, a broken stream)
// the reference answers VIX_E_HOST_NETWORK_CONN_REFUSED (14009) for every
// case tested (NBD-0002); a refusal by the host keeps its own mapping.
func nbdReadCode(err error) vix.Code {
	if errors.Is(err, nfc.ErrRefused) {
		return vix.ToCode(err)
	}
	return vix.EHostConnRefused
}
