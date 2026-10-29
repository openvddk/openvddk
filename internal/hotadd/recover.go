// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package hotadd

import (
	"context"
	"fmt"
	"os"
	"time"
)

func pid() int { return os.Getpid() }

// Action is what recovery does with one orphaned journal record. The
// decision is made by plan alone, so the administrative dry run (CONTRACT.md
// §104) and the real recovery cannot disagree.
type Action int

const (
	// ActionLeave: the record belongs to another vCenter or another proxy
	// VM (for example inside a cloned proxy); it is not touched.
	ActionLeave Action = iota
	// ActionForget: the transaction never reached the attach; the record is
	// removed.
	ActionForget
	// ActionDetach: the journaled disk is still attached at the journaled
	// controller and unit with the journaled backing; it is detached, its
	// redo log removed if ESXi left it, and the record removed.
	ActionDetach
	// ActionFinish: the disk is no longer attached; leftover redo files are
	// removed and the record removed.
	ActionFinish
	// ActionRefuse: the attached device is still in use on the proxy (a
	// holder or a mount); nothing is done, the record is kept for a person
	// to review.
	ActionRefuse
)

func (a Action) String() string {
	switch a {
	case ActionLeave:
		return "leave"
	case ActionForget:
		return "forget"
	case ActionDetach:
		return "detach"
	case ActionFinish:
		return "finish"
	case ActionRefuse:
		return "refuse"
	}
	return fmt.Sprintf("action(%d)", int(a))
}

// Plan is the decision for one orphaned record.
type Plan struct {
	Action Action
	Reason string
	Disk   ProxyDisk // the attached disk, for ActionDetach and ActionRefuse
}

// plan decides what recovery does with record r. It reads vSphere and the
// proxy's devices but changes nothing.
func plan(ctx context.Context, p Platform, h LocalHost, r Record, proxyVM string) (Plan, error) {
	if r.VCenterUUID != p.VCenterUUID() {
		return Plan{Action: ActionLeave, Reason: "recorded by another vCenter (" + r.VCenterUUID + ")"}, nil
	}
	if proxyVM != "" && r.ProxyVM != proxyVM {
		return Plan{Action: ActionLeave, Reason: "recorded by another proxy VM (" + r.ProxyVM + ")"}, nil
	}
	if !mayHaveAttached(r.State) {
		return Plan{Action: ActionForget, Reason: "never attached (state " + string(r.State) + ")"}, nil
	}
	proxy, err := p.Proxy(ctx, r.ProxyVM)
	if err != nil {
		return Plan{}, err
	}
	d, ok := findAttached(proxy, r)
	if !ok {
		return Plan{Action: ActionFinish, Reason: "no longer attached; leftover redo files removed if any"}, nil
	}
	// The device must not be in use on this host before it is pulled.
	if disks, err := h.Disks(); err == nil {
		for _, ld := range disks {
			if isRecordDevice(ld, r) && (len(ld.Holders) > 0 || h.Mounted()[[2]uint32{ld.Major, ld.Minor}]) {
				return Plan{Action: ActionRefuse, Disk: d,
					Reason: fmt.Sprintf("%s is still claimed on the proxy (%v); manual review required", ld.Name, ld.Holders)}, nil
			}
		}
	}
	return Plan{Action: ActionDetach, Disk: d,
		Reason: fmt.Sprintf("attached at scsi%d:%d (device key %d)", r.BusNumber, r.Unit, d.Key)}, nil
}

// Assessment is the dry-run view of one journal record.
type Assessment struct {
	Rec Record
	// Live: the owning process still holds the record; recovery skips it.
	Live bool
	Plan Plan
	Err  error
}

// Assess reports, for every journal record, what Recover would do now,
// without changing anything (the administrative dry run, CONTRACT.md
// §104). Orphans are locked while they are assessed, as recovery locks them.
func Assess(ctx context.Context, p Platform, h LocalHost, o Options, proxyVM string) ([]Assessment, []error) {
	if o.Journal == nil {
		return nil, nil
	}
	orphans, errs := o.Journal.Orphans()
	seen := map[string]bool{}
	var out []Assessment
	for _, e := range orphans {
		pl, err := plan(ctx, p, h, e.Rec, proxyVM)
		out = append(out, Assessment{Rec: e.Rec, Plan: pl, Err: err})
		seen[e.Rec.ID] = true
		e.Release()
	}
	for _, r := range o.Journal.Active() {
		if !seen[r.ID] {
			out = append(out, Assessment{Rec: r, Live: true})
		}
	}
	return out, errs
}

// Recover reconciles orphaned transactions of this proxy with the actual
// vSphere state (CONTRACT.md section 27). It never assumes the last
// journaled transition completed: it looks for the attachment on the proxy
// and detaches it only when its backing is the journaled source path at the
// journaled controller and unit. Records of another vCenter or another proxy
// VM (for example inside a cloned proxy) are left untouched. Repeated
// recovery is safe.
func Recover(ctx context.Context, p Platform, h LocalHost, o Options, proxyVM string) (cleaned, remaining int, errs []error) {
	if o.Journal == nil {
		return 0, 0, nil
	}
	orphans, lerrs := o.Journal.Orphans()
	errs = append(errs, lerrs...)
	for _, e := range orphans {
		r := e.Rec
		pl, err := plan(ctx, p, h, r, proxyVM)
		if err == nil && pl.Action == ActionLeave {
			e.Release()
			remaining++
			continue
		}
		if err == nil {
			err = recoverOne(ctx, p, h, o, e, pl)
		} else {
			keep(e, err)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("journal %s: %w", r.ID, err))
			remaining++
			continue
		}
		cleaned++
		if pl.Action != ActionForget {
			o.logf("HotAdd: recovered transaction %s (%s at scsi%d:%d)", r.ID, r.SourcePath, r.BusNumber, r.Unit)
		}
	}
	return cleaned, remaining, errs
}

// RecoverID finishes one transaction of this process whose detach failed
// (for example because the vCenter session had expired, A4): the same
// reconciliation as Recover, with a new session, for that record only.
// Other records are left to Recover and Cleanup.
func RecoverID(ctx context.Context, p Platform, h LocalHost, o Options, id string) error {
	if o.Journal == nil {
		return fmt.Errorf("journal %s: no journal", id)
	}
	orphans, errs := o.Journal.Orphans()
	var found *Entry
	for _, e := range orphans {
		if e.Rec.ID == id {
			found = e
			continue
		}
		e.Release()
	}
	if found == nil {
		return fmt.Errorf("journal %s: not available for recovery (%v)", id, errs)
	}
	pl, err := plan(ctx, p, h, found.Rec, "")
	if err != nil {
		return fmt.Errorf("journal %s: %w", id, keep(found, err))
	}
	if err := recoverOne(ctx, p, h, o, found, pl); err != nil {
		return fmt.Errorf("journal %s: %w", id, err)
	}
	r := found.Rec
	o.logf("HotAdd: recovered transaction %s (%s at scsi%d:%d)", r.ID, r.SourcePath, r.BusNumber, r.Unit)
	return nil
}

// recoverOne carries out pl for the locked orphan e. On failure the record
// is kept (with the attempt counted) and released for a later recovery.
func recoverOne(ctx context.Context, p Platform, h LocalHost, o Options, e *Entry, pl Plan) error {
	fail := func(err error) error { return keep(e, err) }
	switch pl.Action {
	case ActionForget:
		e.Rec.State = StateClean
		if err := e.Finish(); err != nil {
			e.Release()
			return err
		}
		return nil
	case ActionRefuse:
		return fail(fmt.Errorf("%w: %s", ErrUnsafe, pl.Reason))
	case ActionDetach:
		e.Rec.DeviceKey = pl.Disk.Key
		learnRedo(ctx, p, &e.Rec, o)
		releaseLocal(h, e.Rec, false, o)
		if err := e.Transition(StateDetachSubmitted); err != nil {
			return fail(err)
		}
		release := holdReads(o, "recovery detach")
		err := p.Detach(ctx, DetachSpec{ProxyVM: e.Rec.ProxyVM, DeviceKey: pl.Disk.Key, ExpectPath: e.Rec.SourcePath})
		release()
		if err != nil {
			return fail(err)
		}
		e.Transition(StateDetachConfirmed)
		waitGone(h, e.Rec, 30*time.Second)
	case ActionFinish:
	default:
		e.Release()
		return fmt.Errorf("unexpected recovery action %v", pl.Action)
	}
	// removeRedoAndFinish counts a failed attempt itself.
	if err := removeRedoAndFinish(ctx, p, e); err != nil {
		e.Release()
		return err
	}
	return nil
}

// keep counts a failed recovery attempt in the record, saves it and
// releases it for a later recovery.
func keep(e *Entry, err error) error {
	e.Rec.CleanupAttempts++
	e.Rec.LastError = err.Error()
	e.Save()
	e.Release()
	return err
}
