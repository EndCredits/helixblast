package worker

import (
	"testing"
)

func TestNewJob(t *testing.T) {
	job := NewJob("blastn", "nt", ">seq1\nATGC", nil)

	if job.ID == "" {
		t.Error("job ID should not be empty")
	}
	if job.Status != StatusPending {
		t.Errorf("initial status should be pending, got %s", job.Status)
	}
	if job.Program != "blastn" {
		t.Errorf("expected program blastn, got %s", job.Program)
	}
	if job.Database != "nt" {
		t.Errorf("expected database nt, got %s", job.Database)
	}
}

func TestJobStateTransitions(t *testing.T) {
	job := NewJob("blastn", "nt", ">seq1\nATGC", nil)

	job.SetStatus(StatusQueued)
	if job.GetStatus() != StatusQueued {
		t.Errorf("expected queued, got %s", job.GetStatus())
	}

	job.SetStatus(StatusRunning)
	if job.GetStatus() != StatusRunning {
		t.Errorf("expected running, got %s", job.GetStatus())
	}

	job.SetStatus(StatusSuccess)
	if job.GetStatus() != StatusSuccess {
		t.Errorf("expected success, got %s", job.GetStatus())
	}
}

func TestJobCancel(t *testing.T) {
	job := NewJob("blastn", "nt", ">seq1\nATGC", nil)

	if job.IsCancelling() {
		t.Error("should not be cancelling initially")
	}

	job.Cancel()

	if !job.IsCancelling() {
		t.Error("should be cancelling after Cancel()")
	}
}

func TestJobSnapshot(t *testing.T) {
	job := NewJob("blastn", "nt", ">seq1\nATGC", nil)
	job.SetStatus(StatusRunning)
	job.SetProgress("BLAST in progress")

	snap := job.Snapshot()

	if snap.ID != job.ID {
		t.Error("snapshot ID should match")
	}
	if snap.Status != StatusRunning {
		t.Errorf("snapshot status should be running, got %s", snap.Status)
	}
	if snap.Progress != "BLAST in progress" {
		t.Errorf("snapshot progress mismatch: %s", snap.Progress)
	}
}

func TestJobIDUnique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewJobID()
		if ids[id] {
			t.Errorf("duplicate job ID: %s", id)
		}
		ids[id] = true
	}
}
