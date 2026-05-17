package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
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
	Databases      []string          `json:"-"`
	FastA          string            `json:"-"`
	AdvancedParams map[string]string `json:"-"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Result         json.RawMessage   `json:"result,omitempty"`
	Error          string            `json:"error,omitempty"`
	Progress       string            `json:"progress,omitempty"`

	mu         sync.RWMutex
	cancelling atomic.Bool
	cancel     context.CancelFunc
	subs       map[chan struct{}]struct{}
}

func NewJobID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return "hxb-" + hex.EncodeToString(b)
}

func NewJob(program string, databases []string, fasta string, advanced map[string]string) *Job {
	now := time.Now()
	dbStr := strings.Join(databases, ",")
	return &Job{
		ID:             NewJobID(),
		Status:         StatusPending,
		Program:        program,
		Database:       dbStr,
		Databases:      databases,
		FastA:          fasta,
		AdvancedParams: advanced,
		CreatedAt:      now,
		UpdatedAt:      now,
		subs:           make(map[chan struct{}]struct{}),
	}
}

func (j *Job) Subscribe() chan struct{} {
	j.mu.Lock()
	defer j.mu.Unlock()
	ch := make(chan struct{}, 1)
	j.subs[ch] = struct{}{}
	return ch
}

func (j *Job) Unsubscribe(ch chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.subs, ch)
}

func (j *Job) notify() {
	j.mu.RLock()
	defer j.mu.RUnlock()
	for ch := range j.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (j *Job) SetStatus(s Status) {
	j.mu.Lock()
	old := j.Status
	j.Status = s
	j.UpdatedAt = time.Now()
	j.mu.Unlock()
	if old != s {
		j.notify()
	}
}

func (j *Job) GetStatus() Status {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.Status
}

func (j *Job) SetProgress(msg string) {
	j.mu.Lock()
	j.Progress = msg
	j.UpdatedAt = time.Now()
	j.mu.Unlock()
	j.notify()
}

func (j *Job) SetError(err string) {
	j.mu.Lock()
	j.Error = err
	j.UpdatedAt = time.Now()
	j.mu.Unlock()
	j.notify()
}

func (j *Job) SetResult(data json.RawMessage) {
	j.mu.Lock()
	j.Result = data
	j.UpdatedAt = time.Now()
	j.mu.Unlock()
	j.notify()
}

func (j *Job) ClearResult() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Result = nil
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
