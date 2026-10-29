// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package hotadd implements the HotAdd transport as a crash-recoverable
// transaction (CONTRACT.md sections 22 to 31, 44, 57, observations
// HOTADD-0001 and HOTADD-0002).
//
// Strategy (DIRECT_ATTACH, as observed in the reference): the snapshot
// point's own disk file is attached to the proxy VM with disk mode
// independent_nonpersistent, so every write the proxy might make lands in a
// throw-away redo log that ESXi creates in the proxy's own directory and
// deletes at detach (its location is journaled so a leftover can be removed;
// HOTADD-0003). The source disk is never written. The attachment is found in
// the guest by WWID, verified, read with pread, and detached on Close.
package hotadd

// State is a transaction state (CONTRACT.md section 24). States from
// AttachIntent onwards are journaled before the action they announce.
type State string

const (
	StateNew                State = "NEW"
	StateSourceResolved     State = "SOURCE_RESOLVED"
	StateProxyValidated     State = "PROXY_VALIDATED"
	StateSlotReserved       State = "SLOT_RESERVED"
	StateDirectAttach       State = "DIRECT_ATTACH_SELECTED"
	StateAttachIntent       State = "ATTACH_INTENT"
	StateAttachSubmitted    State = "ATTACH_SUBMITTED"
	StateAttachConfirmed    State = "ATTACH_CONFIRMED"
	StateGuestDeviceFound   State = "GUEST_DEVICE_IDENTIFIED"
	StateGuestDeviceChecked State = "GUEST_DEVICE_VERIFIED"
	StateActive             State = "ACTIVE"
	StateCloseStarted       State = "CLOSE_STARTED"
	StateDetachIntent       State = "DETACH_INTENT"
	StateDetachSubmitted    State = "DETACH_SUBMITTED"
	StateDetachConfirmed    State = "DETACH_CONFIRMED"
	StateChildRemoveIntent  State = "CHILD_REMOVE_INTENT"
	StateChildRemoved       State = "CHILD_REMOVED"
	StateClean              State = "CLEAN"
)

// mayHaveAttached reports whether a record in state s can have a disk
// attached on the proxy (so recovery must look for one).
func mayHaveAttached(s State) bool {
	switch s {
	case StateNew, StateSourceResolved, StateProxyValidated, StateSlotReserved, StateDirectAttach, StateClean:
		return false
	}
	return true
}
