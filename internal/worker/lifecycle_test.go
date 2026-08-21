package worker

import (
	"sync"
	"testing"
	"time"
)

func TestNewJob(t *testing.T) {
	job := NewJob("blastn", []string{"nt"}, ">seq1\nATGC", nil)

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
	job := NewJob("blastn", []string{"nt"}, ">seq1\nATGC", nil)

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
	job := NewJob("blastn", []string{"nt"}, ">seq1\nATGC", nil)

	if job.IsCancelling() {
		t.Error("should not be cancelling initially")
	}

	job.Cancel()

	if !job.IsCancelling() {
		t.Error("should be cancelling after Cancel()")
	}
}

func TestJobSnapshot(t *testing.T) {
	job := NewJob("blastn", []string{"nt"}, ">seq1\nATGC", nil)
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

func TestJobConcurrentAccess(t *testing.T) {
	job := NewJob("blastn", []string{"nt"}, ">seq1\nATGC", nil)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 200; k++ {
				job.SetQueuePos(k)
				job.SetProgress("working")
				job.SetStatus(StatusRunning)
				_ = job.GetStatus()
				snap := job.Snapshot()
				if snap.QueuePos < 0 {
					t.Error("queue_pos must never be negative")
				}
			}
		}()
	}
	wg.Wait()

	if job.IsCancelling() {
		t.Error("job must not be cancelling")
	}
}

func TestPruneExpired(t *testing.T) {
	now := time.Now()
	p := &Pool{
		jobs:      make(map[string]*Job),
		resultTTL: time.Hour,
	}

	add := func(status Status, updatedAt time.Time) *Job {
		j := NewJob("blastn", []string{"nt"}, ">seq1\nATGC", nil)
		j.Status = status
		j.UpdatedAt = updatedAt
		p.jobs[j.ID] = j
		return j
	}

	oldSuccess := add(StatusSuccess, now.Add(-2*time.Hour))
	oldFailed := add(StatusFailed, now.Add(-3*time.Hour))
	freshSuccess := add(StatusSuccess, now.Add(-time.Minute))
	agedRunning := add(StatusRunning, now.Add(-2*time.Hour))

	removed := p.pruneExpired(now)

	if removed != 2 {
		t.Errorf("pruneExpired() removed %d jobs, want 2", removed)
	}
	if len(p.jobs) != 2 {
		t.Errorf("registry size = %d, want 2", len(p.jobs))
	}

	contains := func(target *Job) bool {
		for _, j := range p.jobs {
			if j == target {
				return true
			}
		}
		return false
	}
	if contains(oldSuccess) || contains(oldFailed) {
		t.Error("aged terminal jobs must be pruned")
	}
	if !contains(freshSuccess) {
		t.Error("fresh terminal job must be kept")
	}
	if !contains(agedRunning) {
		t.Error("running job must never be pruned regardless of age")
	}
}
