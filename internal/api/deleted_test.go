package api

import (
	"testing"
)

func TestProcessingState_InitiallyInactive(t *testing.T) {
	ps := &processingState{}
	status := ps.Status()
	if status.Active {
		t.Error("expected inactive initially")
	}
}

func TestProcessingState_StartAndProgress(t *testing.T) {
	ps := &processingState{}
	if err := ps.Start("restore", 5); err != nil {
		t.Fatal(err)
	}
	status := ps.Status()
	if !status.Active {
		t.Error("expected active")
	}
	if status.Operation != "restore" {
		t.Errorf("got %s", status.Operation)
	}
	if status.Total != 5 {
		t.Errorf("got %d", status.Total)
	}

	ps.Advance()
	ps.Advance()
	status = ps.Status()
	if status.Completed != 2 {
		t.Errorf("got %d completed", status.Completed)
	}
}

func TestProcessingState_Complete(t *testing.T) {
	ps := &processingState{}
	ps.Start("purge", 3)
	ps.Complete()
	status := ps.Status()
	if status.Active {
		t.Error("expected inactive after Complete")
	}
}

func TestProcessingState_RejectsConcurrent(t *testing.T) {
	ps := &processingState{}
	ps.Start("restore", 5)
	if err := ps.Start("purge", 3); err == nil {
		t.Error("expected error for concurrent operation")
	}
}
