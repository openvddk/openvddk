// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/openvddk/openvddk/internal/config"
	"github.com/openvddk/openvddk/internal/nfc"
	"github.com/openvddk/openvddk/internal/vix"
)

// OpenVix.nbd.sessions is 1 unless the file sets 1 to 16 (D-0045); other
// values keep 1. The other NBD settings are read as before.
func TestNBDConfigSessions(t *testing.T) {
	for _, c := range []struct {
		file     string
		sessions int
	}{
		{"", 1},
		{"OpenVix.nbd.sessions = 4\n", 4},
		{"OpenVDDK.nbd.sessions = 16\n", 16},
		{"OpenVix.nbd.sessions = 1\n", 1},
		{"OpenVix.nbd.sessions = 0\n", 1},
		{"OpenVix.nbd.sessions = 17\n", 1},
		{"OpenVix.nbd.sessions = four\n", 1},
	} {
		if got := nbdConfig(config.Parse([]byte(c.file))).sessions; got != c.sessions {
			t.Errorf("%q: %d sessions, want %d", c.file, got, c.sessions)
		}
	}
	st := nbdConfig(config.Parse([]byte("OpenVix.nbd.inFlight = 32\nOpenVix.nbd.readAheadKB = 0\nOpenVix.nbd.sessions = 2\n")))
	if st.inFlight != 32 || st.raOn || st.sessions != 2 || st.ra != nbdDefaults.ra {
		t.Errorf("settings %+v", st)
	}
}

// VMware's documented vixDiskLib.nfcAio.Session keys are read under their
// own names, in any case (D-0048); out-of-range values leave the host's
// default (0).
func TestNBDConfigAIOBuffers(t *testing.T) {
	for _, c := range []struct {
		file        string
		size, count uint32
	}{
		{"", 0, 0},
		{"vixDiskLib.nfcAio.Session.BufSizeIn64KB = 32\nvixDiskLib.nfcAio.Session.BufCount = 4\n", 2 << 20, 4},
		{"VIXDISKLIB.NFCAIO.SESSION.BUFSIZEIN64KB = \"1\"\n", 64 << 10, 0},
		{"vixDiskLib.nfcAio.Session.BufSizeIn64KB = 33\nvixDiskLib.nfcAio.Session.BufCount = 17\n", 0, 0},
		{"vixDiskLib.nfcAio.Session.BufSizeIn64KB = 0\nvixDiskLib.nfcAio.Session.BufCount = x\n", 0, 0},
	} {
		st := nbdConfig(config.Parse([]byte(c.file)))
		if st.aioBuf != c.size || st.aioCount != c.count {
			t.Errorf("%q: buffers %d x %d, want %d x %d", c.file, st.aioBuf, st.aioCount, c.size, c.count)
		}
	}
}

// VMware's vixDiskLib.nfc.ReadTimeoutMs and vixDiskLib.nfcAio.SocketOption
// keys are read under their own names; invalid values keep the defaults
// (D-0049).
func TestNBDConfigTimeoutAndSockets(t *testing.T) {
	st := nbdConfig(config.Parse([]byte("vixDiskLib.nfc.ReadTimeoutMs = 10000\nvixDiskLib.nfcAio.SocketOption.SndBufSize = 262144\nvixDiskLib.nfcAio.SocketOption.RcvBufSize = 393216\nvixDiskLib.nfcAio.SocketOption.ServerSndBufSize = 524288\nvixDiskLib.nfcAio.SocketOption.ServerRcvBufSize = 786432\n")))
	if st.readTimeout != 10*time.Second || st.sndBuf != 262144 || st.rcvBuf != 393216 || st.serverSndBuf != 524288 || st.serverRcvBuf != 786432 {
		t.Errorf("settings %+v", st)
	}
	st = nbdConfig(config.Parse([]byte("vixDiskLib.nfc.ReadTimeoutMs = 0\nvixDiskLib.nfcAio.SocketOption.SndBufSize = -1\nvixDiskLib.nfcAio.SocketOption.ServerRcvBufSize = x\n")))
	if st.readTimeout != 0 || st.sndBuf != 0 || st.serverRcvBuf != 0 {
		t.Errorf("invalid values: %+v", st)
	}
}

// No connection to the host's NFC port is 14009 at Open, as the reference
// answers whether the port refuses or stays silent (NBD-0011, NBD-0028).
func TestNBDOpenCodeNoConnection(t *testing.T) {
	err := fmt.Errorf("NFC session with h: %w", fmt.Errorf("nfc: connect h:902: %w: %w", nfc.ErrConnect, errors.New("i/o timeout")))
	if c := nbdOpenCode(err); c != vix.EHostConnRefused {
		t.Fatalf("code %d, want 14009", int(c))
	}
}
