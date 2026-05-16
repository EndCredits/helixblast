package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSuccess   Status = "success"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Job struct {
	ID             string            `json:"job_id"`
	Status         Status            `json:"status"`
	QueuePos       int               `json:"queue_pos"`
	Program        string            `json:"program"`
	Database       string            `json:"database"`
	FastA          string            `json:"-"`
	AdvancedParams map[string]string `json:"-"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Result         json.RawMessage   `json:"result,omitempty"`
	Error          string            `json:"error,omitempty"`
	Progress       string            `json:"progress,omitempty"`

	mu        sync.RWMutex
	cancelling atomic.Bool
	cancel    context.CancelFunc
}

func NewJobID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return "hxb-" + hex.EncodeToString(b)
}

func NewJob(program, database string, fasta string, advanced map[string]string) *Job {
	now := time.Now()
	return &Job{
		ID:             NewJobID(),
		Status:         StatusPending,
		Program:        program,
		Database:       database,
		FastA:          fasta,
		AdvancedParams: advanced,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func (j *Job) SetStatus(s Status) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = s
	j.UpdatedAt = time.Now()
}

func (j *Job) GetStatus() Status {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.Status
}

func (j *Job) SetProgress(msg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Progress = msg
	j.UpdatedAt = time.Now()
}

func (j *Job) SetError(err string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Error = err
	j.UpdatedAt = time.Now()
}

func (j *Job) SetResult(data json.RawMessage) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Result = data
	j.UpdatedAt = time.Now()
}

func (j *Job) SetCancel(fn context.CancelFunc) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cancel = fn
}

func (j *Job) Cancel() {
	j.cancelling.Store(true)
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.cancel != nil {
		j.cancel()
	}
}

func (j *Job) IsCancelling() bool {
	return j.cancelling.Load()
}

func (j *Job) Snapshot() Job {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return Job{
		ID:        j.ID,
		Status:    j.Status,
		QueuePos:  j.QueuePos,
		Program:   j.Program,
		Database:  j.Database,
		CreatedAt: j.CreatedAt,
		UpdatedAt: j.UpdatedAt,
		Result:    j.Result,
		Error:     j.Error,
		Progress:  j.Progress,
	}
}
