// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/types"
)

func notConnectedTask() error {
	return task.Error{LocalizedMethodFault: &types.LocalizedMethodFault{Fault: &types.HostNotConnected{}}}
}

func TestHostNotConnectedDetected(t *testing.T) {
	if !hostNotConnected(notConnectedTask()) {
		t.Fatal("HostNotConnected in a task error not detected")
	}
	if hostNotConnected(errors.New("other")) {
		t.Fatal("a plain error detected as HostNotConnected")
	}
	if err := withHost(notConnectedTask()); !errors.Is(err, ErrHostNotConnected) {
		t.Fatalf("withHost did not mark the fault: %v", err)
	}
}

func fastRetry(t *testing.T) {
	f, m := hostRetryFirst, hostRetryMax
	hostRetryFirst, hostRetryMax = time.Millisecond, 2*time.Millisecond
	t.Cleanup(func() { hostRetryFirst, hostRetryMax = f, m })
}

func TestUntilHostConnectedRetriesThenSucceeds(t *testing.T) {
	fastRetry(t)
	n := 0
	err := untilHostConnected(context.Background(), time.Second, "test", func() error {
		if n++; n < 3 {
			return notConnectedTask()
		}
		return nil
	}, nil)
	if err != nil || n != 3 {
		t.Fatalf("err %v after %d tries, want nil after 3", err, n)
	}
}

func TestUntilHostConnectedStopsOnOtherErrors(t *testing.T) {
	fastRetry(t)
	n := 0
	other := errors.New("refused")
	if err := untilHostConnected(context.Background(), time.Second, "test", func() error { n++; return other }, nil); !errors.Is(err, other) || n != 1 {
		t.Fatalf("err %v after %d tries, want the other error after 1", err, n)
	}
}

func TestUntilHostConnectedGivesUpAtTheBound(t *testing.T) {
	fastRetry(t)
	err := untilHostConnected(context.Background(), 20*time.Millisecond, "test", notConnectedTask, nil)
	if !errors.Is(err, ErrHostNotConnected) {
		t.Fatalf("err %v, want ErrHostNotConnected", err)
	}
}

func TestUntilHostConnectedSeesAnEarlierTryTakeEffect(t *testing.T) {
	fastRetry(t)
	n := 0
	err := untilHostConnected(context.Background(), time.Second, "test", func() error { n++; return notConnectedTask() }, func() bool { return true })
	if err != nil || n != 1 {
		t.Fatalf("err %v after %d tries, want nil after 1", err, n)
	}
}
