// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fakeCallbacks registers the given levels as consumer callbacks and
// records what each one received.
func fakeCallbacks(t *testing.T, registered ...Level) map[Level][]string {
	t.Helper()
	got := map[Level][]string{}
	has := map[Level]bool{}
	for _, l := range registered {
		has[l] = true
	}
	SetSink(func(l Level, m string) Delivery {
		if !has[l] {
			return NoCallback
		}
		got[l] = append(got[l], m)
		return Delivered
	})
	t.Cleanup(func() { SetSink(nil) })
	return got
}

// ownDir points the own log file at a test directory and starts the files.
func ownDir(t *testing.T, path string) string {
	t.Helper()
	dir := t.TempDir()
	old := tempDir
	tempDir = func() string { return dir }
	if err := Start(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Stop(); tempDir = old })
	return dir
}

func readAll(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A consumer that registers only the log callback still gets warnings,
// marked as such after the function name (D-0022); the reference delivers
// everything on the log callback (LOG-0001).
func TestWarningWithoutWarnCallback(t *testing.T) {
	got := fakeCallbacks(t, Info)
	OpenEx.Warnf("attach failed: %s", "no free slot")
	emit(Panic, Close, "detach failed")
	want := []string{
		"OpenVDDK: VixDiskLib: VixDiskLib_OpenEx: WARNING: attach failed: no free slot\n",
		"OpenVDDK: VixDiskLib: VixDiskLib_Close: PANIC: detach failed\n",
	}
	if strings.Join(got[Info], "") != strings.Join(want, "") || len(got[Warn]) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestPanicGoesToWarnBeforeLog(t *testing.T) {
	got := fakeCallbacks(t, Info, Warn)
	emit(Panic, "", "out of memory")
	if len(got[Warn]) != 1 || got[Warn][0] != Prefix+"PANIC: out of memory\n" || len(got[Info]) != 0 {
		t.Fatalf("got %v", got)
	}
}

// With every callback registered, messages keep their level, no marker is
// added and no own file is created.
func TestAllCallbacksNoOwnFile(t *testing.T) {
	got := fakeCallbacks(t, Info, Warn, Panic)
	dir := ownDir(t, "")
	Infof("banner")
	Open.Warnf("Cannot open disk")
	if len(got[Info]) != 1 || len(got[Warn]) != 1 || strings.Contains(got[Warn][0], "WARNING: ") {
		t.Fatalf("got %v", got)
	}
	if OwnFile() != "" {
		t.Fatalf("own file created: %s", OwnFile())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("own directory created: %v", entries)
	}
}

// Without callbacks, messages go to <tmp>/openvddk-<user>/openvddk-<pid>.log
// with UTC time and level, secrets redacted, in a private directory.
func TestNoCallbacksOwnFile(t *testing.T) {
	fakeCallbacks(t)
	dir := ownDir(t, "")
	Infof("banner")
	ConnectEx.Warnf("sign-in failed password=%s", "hunter2")
	path := OwnFile()
	if filepath.Dir(filepath.Dir(path)) != dir || !strings.HasPrefix(filepath.Base(filepath.Dir(path)), "openvddk-") ||
		filepath.Base(path) != "openvddk-"+strconv.Itoa(os.Getpid())+".log" {
		t.Fatalf("own file at %q", path)
	}
	fi, err := os.Stat(filepath.Dir(path))
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode %v, %v", fi.Mode(), err)
	}
	text := readAll(t, path)
	line := regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z (LOG|WARN) OpenVDDK: VixDiskLib: `)
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) != 2 || !line.MatchString(lines[0]) || !strings.Contains(lines[1], " WARN OpenVDDK: VixDiskLib: VixDiskLib_ConnectEx: sign-in failed") {
		t.Fatalf("own file:\n%s", text)
	}
	if strings.Contains(text, "hunter2") {
		t.Fatal("secret written to the own file")
	}
}

// Messages produced inside a callback stay suppressed: not offered to
// another callback, not written to the own file.
func TestSuppressedNotRerouted(t *testing.T) {
	calls := 0
	SetSink(func(Level, string) Delivery { calls++; return Suppressed })
	t.Cleanup(func() { SetSink(nil) })
	ownDir(t, "")
	Warnf("inside a callback")
	if calls != 1 || OwnFile() != "" {
		t.Fatalf("calls %d, own file %q", calls, OwnFile())
	}
}

// The configured file receives every message, delivered or not.
func TestConfiguredFile(t *testing.T) {
	got := fakeCallbacks(t, Info)
	path := filepath.Join(t.TempDir(), "openvddk.log")
	ownDir(t, path)
	Infof("banner")
	Close.Warnf("detach failed")
	text := readAll(t, path)
	if len(got[Info]) != 2 || !strings.Contains(text, " LOG OpenVDDK: VixDiskLib: banner\n") ||
		!strings.Contains(text, " WARN OpenVDDK: VixDiskLib: VixDiskLib_Close: detach failed\n") || OwnFile() != "" {
		t.Fatalf("callbacks %v, file:\n%s", got, text)
	}
}

// Before Start and after Stop nothing is written to any file.
func TestNoFileOutsideInitExit(t *testing.T) {
	fakeCallbacks(t)
	dir := t.TempDir()
	old := tempDir
	tempDir = func() string { return dir }
	t.Cleanup(func() { tempDir = old })
	Infof("before init")
	if err := Start(""); err != nil {
		t.Fatal(err)
	}
	Stop()
	Infof("after exit")
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files written outside Init/Exit: %v", entries)
	}
}

// A directory that others can write to, or that belongs to someone else,
// is refused: the message is lost rather than written there.
func TestOwnDirectoryMustBePrivate(t *testing.T) {
	fakeCallbacks(t)
	dir := ownDir(t, "")
	if err := os.Mkdir(filepath.Join(dir, "openvddk-"+userName()), 0o777); err != nil {
		t.Fatal(err)
	}
	os.Chmod(filepath.Join(dir, "openvddk-"+userName()), 0o777)
	Infof("banner")
	if OwnFile() != "" {
		t.Fatalf("own file created in a shared directory: %s", OwnFile())
	}
}

// VMware's tmpDirectory places the own log file's directory (D-0049);
// without it the system temporary directory is used.
func TestOwnFileInTmpDirectory(t *testing.T) {
	fakeCallbacks(t)
	ownDir(t, "") // the system temporary directory, for comparison
	Stop()
	tmp := t.TempDir()
	SetOwnDir(tmp)
	t.Cleanup(func() { SetOwnDir("") })
	if err := Start(""); err != nil {
		t.Fatal(err)
	}
	Infof("banner")
	if path := OwnFile(); filepath.Dir(filepath.Dir(path)) != tmp {
		t.Fatalf("own file at %q, want it under %q", path, tmp)
	}
}

// A message VMware's log level keys hold back reaches only the configured
// trace file, never a callback or the own file (D-0049).
func TestHeldBackMessageOnlyInTraceFile(t *testing.T) {
	got := fakeCallbacks(t, Info, Warn)
	trace := filepath.Join(t.TempDir(), "trace.log")
	ownDir(t, trace)
	SetLevels(2, 1)
	t.Cleanup(func() { SetLevels(-1, -1) })
	Infof("general info held back")
	OpenEx.NFC().Infof("nfc info held back")
	OpenEx.NFC().Warnf("nfc warning delivered")
	text := readAll(t, trace)
	for _, s := range []string{"general info held back", "nfc info held back", "nfc warning delivered"} {
		if !strings.Contains(text, s) {
			t.Errorf("trace file lacks %q", s)
		}
	}
	if s := fmt.Sprint(got); strings.Contains(s, "held back") || !strings.Contains(s, "nfc warning delivered") {
		t.Errorf("callbacks got %s", s)
	}
}
