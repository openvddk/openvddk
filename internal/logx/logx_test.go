// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package logx

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	cases := []struct{ in, mustNotContain string }{
		{"login user=admin password=hunter2", "hunter2"},
		{`Password: "s3 cr3t"`, "s3 cr3t"},
		{"Cookie: vmware_soap_session=\"52a1b2c3\"; Path=/", "52a1b2c3"},
		{"ticket=abc123 host=esx1", "abc123"},
	}
	for _, c := range cases {
		out := Redact(c.in)
		if strings.Contains(out, c.mustNotContain) {
			t.Errorf("Redact(%q) = %q still contains secret", c.in, out)
		}
	}
	if got := Redact("read 2048 sectors at 0"); got != "read 2048 sectors at 0" {
		t.Errorf("benign text altered: %q", got)
	}
}

func TestSink(t *testing.T) {
	var got []string
	SetSink(func(l Level, m string) Delivery { got = append(got, m); return Delivered })
	defer SetSink(nil)
	Warnf("password=%s", "x1")
	if len(got) != 1 || !strings.HasPrefix(got[0], Prefix) || strings.Contains(got[0], "x1") {
		t.Fatalf("unexpected sink output %q", got)
	}
	SetSink(nil)
	Infof("dropped")
	if len(got) != 1 {
		t.Fatal("message delivered after sink removal")
	}
}

// Every line names the implementation and the reference's component; lines
// of an API call also name the function, as the reference's do (D-0021).
func TestLineFormat(t *testing.T) {
	var got []string
	SetSink(func(l Level, m string) Delivery { got = append(got, m); return Delivered })
	defer SetSink(nil)
	OpenEx.Infof("Open disk '%s'", "[ds] vm/vm.vmdk")
	Infof("Disk IO Count: %d", 7)
	Open.Warnf("Cannot open disk %s. Error %d", "[ds] x.vmdk", 13)
	want := []string{
		"OpenVDDK: VixDiskLib: VixDiskLib_OpenEx: Open disk '[ds] vm/vm.vmdk'\n",
		"OpenVDDK: VixDiskLib: Disk IO Count: 7\n",
		"OpenVDDK: VixDiskLib: VixDiskLib_Open: Cannot open disk [ds] x.vmdk. Error 13\n",
	}
	if len(got) != len(want) {
		t.Fatalf("got %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// VMware's log level keys hold messages back by level (D-0049): without
// them everything is delivered; at error or warning only warnings; at
// audit/info and above everything. NFC messages follow nfc.LogLevel, the
// others transport.LogLevel.
func TestLevels(t *testing.T) {
	t.Cleanup(func() { SetLevels(-1, -1) })
	for _, c := range []struct {
		transport, nfc int
		want           [4]bool // general info, general warn, NFC info, NFC warn
	}{
		{-1, -1, [4]bool{true, true, true, true}},
		{3, 1, [4]bool{true, true, false, true}},
		{2, 3, [4]bool{false, true, true, true}},
		{0, 0, [4]bool{false, false, false, false}},
	} {
		SetLevels(c.transport, c.nfc)
		got := [4]bool{delivered(generalChannel, Info), delivered(generalChannel, Warn), delivered(nfcChannel, Info), delivered(nfcChannel, Warn)}
		if got != c.want {
			t.Errorf("levels %d/%d: delivered %v, want %v", c.transport, c.nfc, got, c.want)
		}
		if !delivered(generalChannel, Panic) || !delivered(nfcChannel, Panic) {
			t.Errorf("levels %d/%d: a panic was held back", c.transport, c.nfc)
		}
	}
}
