// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package logx routes library log messages to the consumer's InitEx
// callbacks (through the C boundary) and redacts secrets before they leave
// the process boundary (CONTRACT.md sections 14 and 55). A message whose
// callback is missing goes to the next callback, marked with its level; a
// message no callback takes goes to the library's own log file (D-0022).
package logx

import (
	"fmt"
	"regexp"
	"sync/atomic"
)

// Level selects the consumer callback: 0 log, 1 warn, 2 panic.
type Level int

const (
	Info  Level = 0
	Warn  Level = 1
	Panic Level = 2
)

// Prefix starts every message: the implementation's name, so logs make
// clear which library produced them (CONTRACT.md section 86), then the
// reference's component name, so a message about the same step is found by
// the same search as the reference's (D-0021).
const Prefix = "OpenVDDK: VixDiskLib: "

// Fn is the VixDiskLib function a message belongs to; its messages carry
// the function's name after Prefix, as the reference's do
// ("VixDiskLib: VixDiskLib_OpenEx: …"). Steps inside a call (attach, guest
// discovery, detach, recovery) belong to that call (D-0021).
type Fn string

// The functions messages are logged under. OpenEx is the reference's log
// name for the Open call; Open is the name it uses for Open's failure line.
const (
	ConnectEx            Fn = "VixDiskLib_ConnectEx"
	Disconnect           Fn = "VixDiskLib_Disconnect"
	OpenEx               Fn = "VixDiskLib_OpenEx"
	Open                 Fn = "VixDiskLib_Open"
	Close                Fn = "VixDiskLib_Close"
	Read                 Fn = "VixDiskLib_Read"
	GetInfo              Fn = "VixDiskLib_GetInfo"
	GetTransportMode     Fn = "VixDiskLib_GetTransportMode"
	QueryAllocatedBlocks Fn = "VixDiskLib_QueryAllocatedBlocks"
	GetMetadataKeys      Fn = "VixDiskLib_GetMetadataKeys"
	ReadMetadata         Fn = "VixDiskLib_ReadMetadata"
	PrepareForAccess     Fn = "VixDiskLib_PrepareForAccess"
	EndAccess            Fn = "VixDiskLib_EndAccess"
	Cleanup              Fn = "VixDiskLib_Cleanup"
	FreeConnectParams    Fn = "VixDiskLib_FreeConnectParams"
	Exit                 Fn = "VixDiskLib_Exit"
)

// Infof logs an informational message under the function f.
func (f Fn) Infof(format string, args ...any) { emit(Info, f, format, args...) }

// Warnf logs a warning under the function f.
func (f Fn) Warnf(format string, args ...any) { emit(Warn, f, format, args...) }

// NFC is f for a message about the NFC transports (NBDSSL, NBD), which
// VMware's vixDiskLib.nfc.LogLevel governs rather than
// vixDiskLib.transport.LogLevel (D-0049).
func (f Fn) NFC() NFCFn { return NFCFn(f) }

// NFCFn logs NFC messages under a function.
type NFCFn Fn

// Infof logs an informational NFC message.
func (f NFCFn) Infof(format string, args ...any) { emitIn(nfcChannel, Info, Fn(f), format, args...) }

// Warnf logs an NFC warning.
func (f NFCFn) Warnf(format string, args ...any) { emitIn(nfcChannel, Warn, Fn(f), format, args...) }

type channel int

const (
	generalChannel channel = iota
	nfcChannel
)

// levels holds VMware's vixDiskLib.transport.LogLevel (general messages)
// and vixDiskLib.nfc.LogLevel (NFC messages) when the configuration sets
// them, -1 otherwise (D-0049).
var levels [2]atomic.Int32

func init() { SetLevels(-1, -1) }

// SetLevels applies VMware's log level keys; -1 means not set, and every
// message is delivered as before. transport.LogLevel: 0 panic, 1 error,
// 2 warning, 3 audit and above; nfc.LogLevel: 0 quiet, 1 error, 2 warning,
// 3 info and above. This library has two levels of its own: a warning is
// delivered from level 1 (error) up, an informational message from level 3
// up; a panic always. A message held back still goes to the configured
// trace file.
func SetLevels(transport, nfc int) {
	levels[generalChannel].Store(int32(transport))
	levels[nfcChannel].Store(int32(nfc))
}

func delivered(ch channel, l Level) bool {
	set := levels[ch].Load()
	switch {
	case set < 0 || l == Panic:
		return true
	case l == Warn:
		return set >= 1
	default:
		return set >= 3
	}
}

// Delivery is what the sink did with a message.
type Delivery int

const (
	// Delivered: the callback of the level took the message.
	Delivered Delivery = iota
	// NoCallback: the consumer registered no callback for the level.
	NoCallback
	// Suppressed: dropped on purpose (a message produced inside a
	// callback); it is not offered to another callback.
	Suppressed
)

type sinkFunc func(Level, string) Delivery

var sink atomic.Pointer[sinkFunc]

// SetSink installs the delivery function. A nil sink delivers nothing.
func SetSink(f func(Level, string) Delivery) {
	if f == nil {
		sink.Store(nil)
		return
	}
	s := sinkFunc(f)
	sink.Store(&s)
}

// route lists the callbacks a message of each level is offered to, in
// order. The reference delivers every message on the log callback, so a
// consumer that registers only that one sees all of the reference's
// messages; ours must not lose warnings there either (LOG-0001, D-0022).
var route = [...][]Level{
	Info:  {Info},
	Warn:  {Warn, Info},
	Panic: {Panic, Warn, Info},
}

// marks start our text when a message is delivered on another level's
// callback (part 3 of the D-0021 format).
var marks = [...]string{Info: "", Warn: "WARNING: ", Panic: "PANIC: "}

func emit(l Level, fn Fn, format string, args ...any) {
	emitIn(generalChannel, l, fn, format, args...)
}

func emitIn(ch channel, l Level, fn Fn, format string, args ...any) {
	head := Prefix
	if fn != "" {
		head += string(fn) + ": "
	}
	text := Redact(fmt.Sprintf(format, args...))
	if !delivered(ch, l) {
		files.write(l, head+text, true) // the trace file only
		return
	}
	taken := false
	if s := sink.Load(); s != nil {
		for _, to := range route[l] {
			mark := ""
			if to != l {
				mark = marks[l]
			}
			if d := (*s)(to, head+mark+text+"\n"); d != NoCallback {
				taken = true
				break
			}
		}
	}
	files.write(l, head+text, taken)
}

// Infof logs an informational message that belongs to no single function
// (start-up, configuration, the per-disk and connection summaries).
func Infof(format string, args ...any) { emit(Info, "", format, args...) }

// Warnf logs a warning that belongs to no single function.
func Warnf(format string, args ...any) { emit(Warn, "", format, args...) }

var secretPatterns = []*regexp.Regexp{
	// key=value and key: value forms of common secret-bearing names
	regexp.MustCompile(`(?i)\b(password|passwd|pwd|cookie|ticket|token|secret|session(?:id)?|key)\b(\s*[=:]\s*)("[^"]*"|\S+)`),
	// vmware_soap_session and similar cookies
	regexp.MustCompile(`(?i)(vmware_soap_session=)("[^"]*"|[^;\s]+)`),
}

// Redact removes values that look like credentials or session material.
func Redact(s string) string {
	s = secretPatterns[0].ReplaceAllString(s, "$1$2<redacted>")
	s = secretPatterns[1].ReplaceAllString(s, "$1<redacted>")
	return s
}
