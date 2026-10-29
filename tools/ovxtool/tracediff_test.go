// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
)

const traceRef = `# vixtrace real=/opt/x/lib64/libvixDiskLib.so.8.0.3 pid=1
1 10 InitEx 8.0 log=set warn=set panic=set libDir="/opt/x" config="(null)" = 0 t=0.010000
2 10 AllocateConnectParams = P:0x5551 t=0.000001
3 10 ConnectEx vmxSpec="moref=vm-1" server="vc" thumbprint=set credType=1 user=set port=0 nfcHostPort=0 specType=0 readOnly=1 snapshot="snapshot-1" transports="hotadd" = 0 C:0x7771 t=1.000000
4 10 Open C:0x7771 path="[ds] a/a.vmdk" flags=0x5 = 0 H:0x9991 t=20.000000
5 11 Read H:0x9991 start=0 n=2048 = 0 hash=00000000000000aa t=0.010000
6 12 Read H:0x9991 start=2048 n=2048 = 0 hash=00000000000000bb t=0.010000
7 10 Close H:0x9991 = 0 t=12.000000
8 10 Open C:0x7771 path="[ds] a/b.vmdk" flags=0x5 = 0 H:0x9991 t=20.000000
9 10 Close H:0x9991 = 0 t=1.000000
`

// The candidate: other addresses and times, reads in another order, a new
// handle value for the second Open, and one read with other data.
const traceCand = `# vixtrace real=/home/u/cand/libvixDiskLib.so.8.0.3 pid=2
1 20 InitEx 8.0 log=set warn=set panic=set libDir="/home/u/cand" config="(null)" = 0 t=0.020000
2 20 AllocateConnectParams = P:0x1 t=0.000001
3 20 ConnectEx vmxSpec="moref=vm-1" server="vc" thumbprint=set credType=1 user=set port=0 nfcHostPort=0 specType=0 readOnly=1 snapshot="snapshot-1" transports="hotadd" = 0 C:0x2 t=0.100000
4 20 Open C:0x2 path="[ds] a/a.vmdk" flags=0x5 = 0 H:0x3 t=2.000000
5 22 Read H:0x3 start=2048 n=2048 = 0 hash=00000000000000bb t=0.001000
6 21 Read H:0x3 start=0 n=2048 = 0 hash=00000000000000ff t=0.001000
7 20 Close H:0x3 = 0 t=0.500000
8 20 Open C:0x2 path="[ds] a/b.vmdk" flags=0x5 = 0 H:0x4 t=2.000000
9 20 Close H:0x4 = 0 t=0.500000
`

func TestTraceDiff(t *testing.T) {
	a, err := parseTrace(strings.NewReader(traceRef))
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseTrace(strings.NewReader(traceCand))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if !compareTraces(&out, a, b, "reference", "candidate") {
		t.Fatalf("a data difference went unreported:\n%s", out.String())
	}
	s := out.String()
	for _, want := range []string{"Control calls: 7 in reference, 7 in candidate; 0 differ.", "different data 1."} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	same, _ := parseTrace(strings.NewReader(strings.Replace(traceCand, "00000000000000ff", "00000000000000aa", 1)))
	out.Reset()
	if compareTraces(&out, a, same, "reference", "candidate") {
		t.Fatalf("equal traces reported as different:\n%s", out.String())
	}
}
