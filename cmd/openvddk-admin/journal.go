// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/openvddk/openvddk/internal/hotadd"
	"github.com/openvddk/openvddk/internal/linuxdev"
)

// ownerStatus says whether each record's owning process still holds it,
// by the library's own test (an orphan is a record whose lock is free).
func ownerStatus(j *hotadd.Journal, proc, bootID string) (map[string]string, []error) {
	orphans, errs := j.Orphans()
	out := map[string]string{}
	for _, e := range orphans {
		r := e.Rec
		switch {
		case bootID != "" && r.BootID != "" && r.BootID != bootID:
			out[r.ID] = "orphan (written before the worker last booted)"
		case !pidRunning(proc, r.PID):
			out[r.ID] = fmt.Sprintf("orphan (process %d has ended)", r.PID)
		default:
			out[r.ID] = fmt.Sprintf("orphan (released by process %d for recovery)", r.PID)
		}
		e.Release()
	}
	for _, r := range j.Active() {
		if _, ok := out[r.ID]; !ok {
			out[r.ID] = fmt.Sprintf("owner alive (process %d)", r.PID)
		}
	}
	return out, errs
}

func pidRunning(proc string, pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.Stat(fmt.Sprintf("%s/%d", proc, pid))
	return err == nil
}

func sortedRecords(j *hotadd.Journal) []hotadd.Record {
	rs := j.Active()
	sort.Slice(rs, func(a, b int) bool { return rs[a].Created.Before(rs[b].Created) })
	return rs
}

func when(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

// writeRecord prints one record in full.
func writeRecord(w io.Writer, r hotadd.Record, status string) {
	fmt.Fprintf(w, "Record %s: state %s, %s\n", r.ID, r.State, status)
	src := r.SourcePath
	if src == "" {
		src = "-"
	}
	fmt.Fprintf(w, "  source     %s (VM %s, snapshot %s, disk key %d, %s)\n", src, dash(r.SourceVM), dash(r.SourceSnapshot), r.SourceDiskKey, gib(r.CapacityBytes))
	fmt.Fprintf(w, "  worker     VM %s, scsi%d:%d (controller key %d)", dash(r.ProxyVM), r.BusNumber, r.Unit, r.ControllerKey)
	if r.DeviceKey != 0 {
		fmt.Fprintf(w, ", device key %d", r.DeviceKey)
	}
	fmt.Fprintln(w)
	if r.LocalDevice != "" || r.LocalWWID != "" {
		fmt.Fprintf(w, "  guest      %s %s (identified by %s)\n", dash(r.LocalDevice), dash(r.LocalWWID), idMode(r))
	}
	fmt.Fprintf(w, "  vCenter    %s (instance %s)\n", dash(r.Server), dash(r.VCenterUUID))
	if r.RedoPath != "" {
		fmt.Fprintf(w, "  redo log   %s\n", r.RedoPath)
	}
	fmt.Fprintf(w, "  process    %d, boot %s\n", r.PID, dash(r.BootID))
	fmt.Fprintf(w, "  created    %s, updated %s\n", when(r.Created), when(r.Updated))
	if r.CleanupAttempts > 0 || r.LastError != "" {
		fmt.Fprintf(w, "  recovery   %d failed attempts; last error: %s\n", r.CleanupAttempts, dash(r.LastError))
	}
}

func idMode(r hotadd.Record) string {
	if r.DeviceHasID {
		return "disk ID"
	}
	return "position"
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func gib(n int64) string {
	if n <= 0 {
		return "size unknown"
	}
	return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
}

// cmdJournal lists the journal without signing in anywhere.
func cmdJournal(o options, w io.Writer) error {
	jdir, _, err := journalDirOf(o)
	if err != nil {
		return err
	}
	j, err := openJournal(jdir)
	if os.IsNotExist(err) {
		fmt.Fprintf(w, "Journal %s does not exist: this worker has never attached a disk with OpenVDDK.\n", jdir)
		return nil
	}
	if err != nil {
		return fmt.Errorf("journal %s: %w", jdir, err)
	}
	return writeJournal(w, j, jdir, linuxdev.System.Proc, linuxdev.System.BootID())
}

func writeJournal(w io.Writer, j *hotadd.Journal, jdir, proc, bootID string) error {
	status, errs := ownerStatus(j, proc, bootID)
	rs := sortedRecords(j)
	fmt.Fprintf(w, "Journal %s: %d record%s.\n", jdir, len(rs), plural(len(rs)))
	for _, r := range rs {
		fmt.Fprintln(w)
		writeRecord(w, r, status[r.ID])
	}
	for _, e := range errs {
		fmt.Fprintf(w, "\nUnreadable: %v\n", e)
	}
	if len(rs) == 0 && len(errs) == 0 {
		fmt.Fprintln(w, "Nothing is attached by OpenVDDK on this worker, and nothing is left to recover.")
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// cmdAttachments lists the worker VM's disks as vCenter sees them, matched
// to the journal and to the guest's devices.
func cmdAttachments(ctx context.Context, wk *worker, w io.Writer) error {
	return writeAttachments(w, wk.proxy, wk.journal, wk.host)
}

func writeAttachments(w io.Writer, p hotadd.ProxyInfo, j *hotadd.Journal, h hotadd.LocalHost) error {
	ids := "no (attached disks are identified by position)"
	if p.DiskIDs {
		ids = "yes"
	}
	fmt.Fprintf(w, "Worker VM %s, recognised by %s; disk IDs in the guest: %s.\n", p.VM, p.Identity, ids)
	var ctl []string
	for _, c := range p.Controllers {
		ctl = append(ctl, fmt.Sprintf("scsi%d %s (key %d)", c.Bus, c.Kind, c.Key))
	}
	fmt.Fprintf(w, "SCSI controllers: %s.\n", strings.Join(ctl, ", "))

	local, _ := h.Disks()
	buses := h.SCSIBuses()
	records := j.Active()
	matched := map[string]bool{}
	var own, attached []hotadd.ProxyDisk
	for _, d := range p.Disks {
		if d.DiskMode == "independent_nonpersistent" && !strings.HasPrefix(d.Path, p.DirDS+"/") {
			attached = append(attached, d)
		} else {
			own = append(own, d)
		}
	}
	fmt.Fprintf(w, "\nThe worker's own disks: %d.\n", len(own))
	for _, d := range own {
		fmt.Fprintf(w, "  scsi%d:%d  %s  %s  %s  guest %s\n", busOf(p, d.ControllerKey), d.Unit, d.Path, gib(d.CapacityBytes), d.DiskMode, guestDevice(d, busOf(p, d.ControllerKey), local, buses))
	}
	fmt.Fprintf(w, "\nSource disks attached to the worker: %d.\n", len(attached))
	for _, d := range attached {
		bus := busOf(p, d.ControllerKey)
		who := "no journal record on this worker: attached by another program or library, or by a worker whose journal is elsewhere; OpenVDDK will not touch it"
		for _, r := range records {
			if r.ControllerKey == d.ControllerKey && r.Unit == d.Unit && r.SourcePath == d.Path {
				who = fmt.Sprintf("journal record %s (state %s)", r.ID, r.State)
				matched[r.ID] = true
			}
		}
		fmt.Fprintf(w, "  scsi%d:%d  %s  %s  guest %s\n            %s\n", bus, d.Unit, d.Path, gib(d.CapacityBytes), guestDevice(d, bus, local, buses), who)
	}
	var loose []hotadd.Record
	for _, r := range records {
		if !matched[r.ID] && r.ProxyVM == p.VM {
			loose = append(loose, r)
		}
	}
	if len(loose) > 0 {
		fmt.Fprintf(w, "\nJournal records whose disk is not attached: %d (recovery finishes them).\n", len(loose))
		for _, r := range loose {
			fmt.Fprintf(w, "  %s  %s  state %s\n", r.ID, dash(r.SourcePath), r.State)
		}
	}
	return nil
}

// guestDevice finds the guest's block device for a disk of the worker VM:
// by disk ID when the disk presents one, otherwise by SCSI position.
func guestDevice(d hotadd.ProxyDisk, bus int32, local []linuxdev.Disk, buses map[int]int32) string {
	if d.UUID != "" {
		if wwid, err := linuxdev.WWIDForBackingUUID(d.UUID); err == nil {
			for _, l := range local {
				if l.WWID == wwid {
					return "/dev/" + l.Name + " (by disk ID)"
				}
			}
		}
	}
	for _, l := range local {
		if b, ok := buses[l.HCTL.Host]; ok && b == bus && int32(l.HCTL.Target) == d.Unit && l.HCTL.LUN == 0 {
			return "/dev/" + l.Name + " (by position)"
		}
	}
	return "not visible"
}

// cmdReconcile is the dry run: what recovery would do, nothing changed.
func cmdReconcile(ctx context.Context, wk *worker, w io.Writer) error {
	as, errs := hotadd.Assess(ctx, wk.plat, wk.host, hotadd.Options{Journal: wk.journal}, wk.proxy.VM)
	writeAssessment(w, as, errs)
	return nil
}

func writeAssessment(w io.Writer, as []hotadd.Assessment, errs []error) {
	count := map[string]int{}
	for _, a := range as {
		r := a.Rec
		head := fmt.Sprintf("Record %s (%s at scsi%d:%d, state %s)", r.ID, dash(r.SourcePath), r.BusNumber, r.Unit, r.State)
		switch {
		case a.Live:
			fmt.Fprintf(w, "%s: owner alive (process %d); recovery skips it.\n", head, r.PID)
			count["skipped, owner alive"]++
		case a.Err != nil:
			fmt.Fprintf(w, "%s: cannot decide: %v\n", head, a.Err)
			count["undecided"]++
		default:
			fmt.Fprintf(w, "%s: %s: %s.\n", head, a.Plan.Action, a.Plan.Reason)
			count[a.Plan.Action.String()]++
		}
	}
	for _, e := range errs {
		fmt.Fprintf(w, "Unreadable record: %v\n", e)
		count["unreadable, kept"]++
	}
	if len(as)+len(errs) == 0 {
		fmt.Fprintln(w, "The journal is empty: nothing to recover.")
		return
	}
	var parts []string
	for k, n := range count {
		parts = append(parts, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(parts)
	fmt.Fprintf(w, "\nSummary: %s.\nNothing was changed. \"openvddk-admin cleanup\" with the same options carries this out.\n", strings.Join(parts, ", "))
}

// cmdCleanup runs the library's recovery for this worker. It reports
// whether records remain.
func cmdCleanup(ctx context.Context, wk *worker, w io.Writer) (bool, error) {
	logf := func(format string, args ...any) {
		fmt.Fprintf(w, "%s %s\n", time.Now().UTC().Format("15:04:05"), fmt.Sprintf(format, args...))
	}
	cleaned, remaining, errs := hotadd.Recover(ctx, wk.plat, wk.host, hotadd.Options{Journal: wk.journal, Logf: logf}, wk.proxy.VM)
	for _, e := range errs {
		fmt.Fprintf(w, "Not recovered: %v\n", e)
	}
	fmt.Fprintf(w, "Recovered %d record%s; %d remain%s.\n", cleaned, plural(cleaned), remaining, map[bool]string{true: "s", false: ""}[remaining == 1])
	return remaining > 0 || len(errs) > 0, nil
}
