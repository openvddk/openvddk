// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	c := Parse([]byte(strings.Join([]string{
		"# comment",
		"",
		`tmpDirectory = "/var/tmp/vddk"`,
		"vixDiskLib.transport.LogLevel = 3\r",
		`  quoted.Inner = "a = b"  `,
		"no-equals-here",
		`bad = "unterminated`,
		"OpenVix.log.level = debug",
		"OpenVix.unknown = 1",
		"TMPDIRECTORY = /override",
	}, "\n")))

	cases := map[string]string{
		"tmpdirectory":                  "/override",
		"vixdisklib.transport.loglevel": "3",
		"VIXDISKLIB.TRANSPORT.LOGLEVEL": "3",
		"quoted.inner":                  "a = b",
		"openvix.log.level":             "debug",
	}
	for k, want := range cases {
		got, ok := c.Get(k)
		if !ok || got != want {
			t.Errorf("Get(%q) = %q,%v want %q", k, got, ok, want)
		}
	}
	if _, ok := c.Get("bad"); ok {
		t.Errorf("unterminated value must be rejected")
	}
	if len(c.Warnings) != 4 {
		t.Errorf("warnings = %d %q, want 4 (no '=', unterminated, unknown own key, override)", len(c.Warnings), c.Warnings)
	}
}

func TestLoadLimits(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(filepath.Join(dir, "missing.conf")); err == nil {
		t.Fatal("missing file must be an error")
	}
	big := filepath.Join(dir, "big.conf")
	if err := os.WriteFile(big, make([]byte, MaxFileSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(big); err == nil {
		t.Fatal("oversized file must be an error")
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("a = b\n# c\n\"x\" = \"y\"\n=\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		c := Parse(data)
		for _, k := range c.Keys() {
			if _, ok := c.Get(k); !ok {
				t.Fatalf("listed key %q not retrievable", k)
			}
		}
	})
}

// Keys written with the public name are the same keys (D-0021).
func TestPublicNamePrefix(t *testing.T) {
	c := Parse([]byte("OpenVDDK.hotadd.readAheadKB = 0\nOPENVDDK.hotadd.nonsense = 1\n"))
	if v, ok := c.Get("OpenVix.hotadd.readAheadKB"); !ok || v != "0" {
		t.Fatalf("OpenVix spelling: %q %v", v, ok)
	}
	if v, ok := c.Get("OpenVDDK.hotadd.readaheadkb"); !ok || v != "0" {
		t.Fatalf("OpenVDDK spelling: %q %v", v, ok)
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "unknown project key") {
		t.Fatalf("warnings %q", c.Warnings)
	}
	d := Parse([]byte("OpenVix.hotadd.readAheadKB = 0\nOpenVDDK.hotadd.readAheadKB = 4096\n"))
	if v, _ := d.Get("OpenVix.hotadd.readAheadKB"); v != "4096" || len(d.Warnings) != 1 {
		t.Fatalf("both spellings: value %q, warnings %q", v, d.Warnings)
	}
	// The NBD keys (D-0037) are project keys, not typos.
	e := Parse([]byte("OpenVix.nbd.inFlight = 32\nOpenVDDK.nbd.readAheadKB = 4096\nOpenVix.nbd.readAheadDepth = 4\nOpenVix.nbd.sessions = 4\n"))
	if v, _ := e.Get("OpenVix.nbd.inFlight"); v != "32" || len(e.Warnings) != 0 {
		t.Fatalf("NBD keys: value %q, warnings %q", v, e.Warnings)
	}
}

// Outside double quotes '#' starts a comment, as VMware's guide describes;
// inside quotes it is part of the value (D-0048).
func TestTrailingComment(t *testing.T) {
	c := Parse([]byte("vixDiskLib.nfcAio.Session.BufCount = 4 # tested best\nOpenVix.log.level = \"a#b\" # note\nx=# all comment\n"))
	if v, _ := c.Get("vixDiskLib.nfcAio.Session.BufCount"); v != "4" {
		t.Errorf("unquoted value with a comment: %q", v)
	}
	if v, _ := c.Get("OpenVix.log.level"); v != "a#b" {
		t.Errorf("quoted value: %q", v)
	}
	if v, ok := c.Get("x"); !ok || v != "" {
		t.Errorf("value that is only a comment: %q %v", v, ok)
	}
}

// Keys VMware documents but the library does not act on are reported with
// the reason, as written in the file (D-0048); honoured and own keys are not.
func TestNotHonoured(t *testing.T) {
	c := Parse([]byte("vixDiskLib.phoneHome.ProductName = x\nvixDiskLib.ssl.enableSslFIPS = 1\nvixDiskLib.nfcAio.Session.BufSizeIn64KB = 16\nOpenVix.nbd.sessions = 2\ntmpDirectory = /var/tmp\nvixDiskLib.transport.san.allowList = /dev/sdb\n"))
	got := c.NotHonoured()
	if len(got) != 3 || got[0][0] != "vixDiskLib.phoneHome.ProductName" || got[1][0] != "vixDiskLib.ssl.enableSslFIPS" || got[2][0] != "vixDiskLib.transport.san.allowList" {
		t.Fatalf("not honoured: %q", got)
	}
	for _, k := range got {
		if k[1] == "" {
			t.Errorf("%s: no reason", k[0])
		}
	}
}
