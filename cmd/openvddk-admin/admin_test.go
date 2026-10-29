// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openvddk/openvddk/internal/hotadd"
	"github.com/openvddk/openvddk/internal/linuxdev"
)

func TestUsageAndExitCodes(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := run(nil, &out, &errb); rc != 2 || !strings.Contains(out.String(), "Commands:") {
		t.Fatalf("no arguments: rc=%d out=%q", rc, out.String())
	}
	if rc := run([]string{"version"}, &out, &errb); rc != 0 || !strings.Contains(out.String(), "openvddk-admin ") {
		t.Fatalf("version: rc=%d", rc)
	}
	if rc := run([]string{"bogus"}, &out, &errb); rc != 2 {
		t.Fatalf("unknown command: rc=%d", rc)
	}
	if rc := run([]string{"journal", "extra"}, &out, &errb); rc != 2 {
		t.Fatalf("stray argument: rc=%d", rc)
	}
	if rc := run([]string{"bundle", "-log-lines", "0"}, &out, &errb); rc != 2 {
		t.Fatalf("bad -log-lines: rc=%d", rc)
	}
}

func TestReadPassword(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pw")
	os.WriteFile(p, []byte("s3cret word\r\nsecond line\n"), 0o600)
	pw, err := readPassword(p)
	if err != nil || string(pw) != "s3cret word" {
		t.Fatalf("%q %v", pw, err)
	}
	os.WriteFile(p, []byte("\n"), 0o600)
	if _, err := readPassword(p); err == nil {
		t.Fatal("an empty password was accepted")
	}
}

// No secret survives into a bundle: the password of the run anywhere, and
// the value of any secret-looking pair (CONTRACT.md §105).
func TestBundleRedaction(t *testing.T) {
	b := newBundle()
	b.file("a.txt").WriteString("login with Pa55-word-x ok\npassword = \"hunter2\"\nvmware_soap_session=\"52a1b2\"; path=/\n")
	b.file("b.txt").WriteString("token: abc123 rest\nsession_key=deadbeef\nnothing secret here: value\n")
	n := b.redact([]byte("Pa55-word-x"))
	all := b.files["a.txt"].String() + b.files["b.txt"].String()
	for _, s := range []string{"Pa55-word-x", "hunter2", "52a1b2", "abc123", "deadbeef"} {
		if strings.Contains(all, s) {
			t.Fatalf("%q survived:\n%s", s, all)
		}
	}
	if n != 5 || !strings.Contains(all, "nothing secret here: value") || !strings.Contains(all, "password = <redacted>") {
		t.Fatalf("n=%d\n%s", n, all)
	}
}

func TestBundleArchive(t *testing.T) {
	b := newBundle()
	b.file("summary.txt").WriteString("hello\n")
	b.file("logs/x.log").WriteString("line\n")
	var buf bytes.Buffer
	if err := b.writeTarGz(&buf, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Mode != 0o600 {
			t.Fatalf("%s mode %o", h.Name, h.Mode)
		}
		names = append(names, h.Name)
	}
	if strings.Join(names, " ") != "openvddk-bundle/summary.txt openvddk-bundle/logs/x.log" {
		t.Fatalf("names %v", names)
	}
}

func TestWriteJournal(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "journal")
	j, err := hotadd.OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	live, _ := j.Begin(hotadd.Record{State: hotadd.StateActive, PID: os.Getpid(), BootID: "boot-1", SourcePath: "[ds] a/a.vmdk", ProxyVM: "vm-9"})
	defer live.Release()
	gone, _ := j.Begin(hotadd.Record{State: hotadd.StateAttachSubmitted, PID: 999999, BootID: "boot-1", SourcePath: "[ds] b/b.vmdk"})
	gone.Release()
	old, _ := j.Begin(hotadd.Record{State: hotadd.StateActive, PID: os.Getpid(), BootID: "boot-0"})
	old.Release()
	var out bytes.Buffer
	writeJournal(&out, j, dir, "/proc", "boot-1")
	s := out.String()
	for _, want := range []string{"3 records", "owner alive (process", "orphan (process 999999 has ended)", "orphan (written before the worker last booted)", "[ds] a/a.vmdk"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

type fakeLocal struct{ disks []linuxdev.Disk }

func (f fakeLocal) ProductUUID() (string, error)                    { return "", nil }
func (f fakeLocal) Disks() ([]linuxdev.Disk, error)                 { return f.disks, nil }
func (f fakeLocal) Rescan() error                                   { return nil }
func (f fakeLocal) Settle()                                         {}
func (f fakeLocal) BootID() string                                  { return "boot-1" }
func (f fakeLocal) Mounted() map[[2]uint32]bool                     { return nil }
func (f fakeLocal) Release(linuxdev.Disk) error                     { return nil }
func (f fakeLocal) PhysicalVolumes(linuxdev.Disk) map[string]string { return nil }
func (f fakeLocal) MACs() []string                                  { return nil }
func (f fakeLocal) SCSIBuses() map[int]int32                        { return map[int]int32{0: 0, 3: 1} }
func (f fakeLocal) Protection(context.Context) linuxdev.Protection {
	return linuxdev.Protection{Safe: true}
}
func (f fakeLocal) Open(linuxdev.Disk, string, int64, bool) (hotadd.Reader, error) {
	return nil, os.ErrNotExist
}

func TestWriteAttachments(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "journal")
	j, _ := hotadd.OpenJournal(dir)
	e, _ := j.Begin(hotadd.Record{State: hotadd.StateActive, ProxyVM: "vm-9", ControllerKey: 1001, Unit: 3, SourcePath: "[ds] src/src.vmdk"})
	e.Release()
	e2, _ := j.Begin(hotadd.Record{State: hotadd.StateDetachSubmitted, ProxyVM: "vm-9", ControllerKey: 1001, Unit: 4, SourcePath: "[ds] gone/gone.vmdk"})
	e2.Release()
	srcWWID, _ := linuxdev.WWIDForBackingUUID("6000C29a-0000-0000-0000-000000000001")
	p := hotadd.ProxyInfo{
		VM: "vm-9", Identity: "disk-ids", DiskIDs: true, DirDS: "[ds] proxy",
		Controllers: []hotadd.Controller{{Key: 1000, Bus: 0, Kind: "pvscsi"}, {Key: 1001, Bus: 1, Kind: "pvscsi"}},
		Disks: []hotadd.ProxyDisk{
			{Key: 2000, ControllerKey: 1000, Unit: 0, Path: "[ds] proxy/proxy.vmdk", DiskMode: "persistent", CapacityBytes: 20 << 30},
			{Key: 2016, ControllerKey: 1001, Unit: 3, Path: "[ds] src/src.vmdk", UUID: "6000C29a-0000-0000-0000-000000000001", DiskMode: "independent_nonpersistent", CapacityBytes: 16 << 30},
			{Key: 2017, ControllerKey: 1001, Unit: 5, Path: "[ds] other/other.vmdk", DiskMode: "independent_nonpersistent", CapacityBytes: 1 << 30},
		},
	}
	h := fakeLocal{disks: []linuxdev.Disk{
		{Name: "sda", HCTL: linuxdev.HCTL{Host: 0, Target: 0}},
		{Name: "sdc", HCTL: linuxdev.HCTL{Host: 3, Target: 3}, WWID: srcWWID},
		{Name: "sdd", HCTL: linuxdev.HCTL{Host: 3, Target: 5}},
	}}
	var out bytes.Buffer
	writeAttachments(&out, p, j, h)
	s := out.String()
	for _, want := range []string{
		"The worker's own disks: 1.", "/dev/sda (by position)",
		"Source disks attached to the worker: 2.", "/dev/sdc (by disk ID)", "journal record " + e.Rec.ID,
		"/dev/sdd (by position)", "no journal record on this worker",
		"whose disk is not attached: 1", e2.Rec.ID,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

// The bundle takes only log files the library wrote: a link planted where
// the library's logs are, or a log in a directory open to others, is
// skipped (SECURITY-REVIEW-0.7.0, 5).
func TestBundleSkipsPlantedLogs(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "openvddk-a")
	open := filepath.Join(root, "openvddk-b")
	for _, d := range []string{good, open} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(open, 0o777); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(root, "secret")
	if err := os.WriteFile(secret, []byte("not for the bundle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "openvddk-1.log"), []byte("library line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(good, "openvddk-2.log")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(open, "openvddk-3.log"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(secret, filepath.Join(good, "openvddk-4.log")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{
		filepath.Join(good, "openvddk-1.log"): true,
		filepath.Join(good, "openvddk-2.log"): false,
		filepath.Join(open, "openvddk-3.log"): false,
		filepath.Join(good, "openvddk-4.log"): false,
	} {
		if got := notLibraryLog(name) == ""; got != want {
			t.Errorf("%s: accepted %v, want %v (%s)", name, got, want, notLibraryLog(name))
		}
	}
	var w bytes.Buffer
	tailFile(&w, filepath.Join(good, "openvddk-2.log"), 10, true)
	if strings.Contains(w.String(), "not for the bundle") {
		t.Fatal("a link was followed")
	}
}
