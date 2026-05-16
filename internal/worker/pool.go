package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/EndCredits/helixblast/internal/blast"
)

type ExecFunc func(ctx context.Context, job *Job) ([]blast.Hit, error)

type Pool struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	jobCh   chan *Job
	maxJobs int
	execFn  ExecFunc
	wg      sync.WaitGroup
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func NewPool(maxConcurrent int, maxQueue int, execFn ExecFunc) *Pool {
	p := &Pool{
		jobs:    make(map[string]*Job),
		jobCh:   make(chan *Job, maxQueue),
		maxJobs: maxConcurrent,
		execFn:  execFn,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}

	for i := 0; i < maxConcurrent; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	return p
}

func (p *Pool) Submit(job *Job) error {
	p.mu.Lock()

	job.SetStatus(StatusPending)

	select {
	case p.jobCh <- job:
		job.SetStatus(StatusQueued)
		job.QueuePos = len(p.jobCh)
		p.jobs[job.ID] = job
		p.mu.Unlock()
		return nil
	default:
		p.mu.Unlock()
		return fmt.Errorf("queue full: max capacity reached (429)")
	}
}

func (p *Pool) Get(id string) (*Job, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	job, ok := p.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return job, nil
}

func (p *Pool) List() []Job {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]Job, 0, len(p.jobs))
	for _, j := range p.jobs {
		snap := j.Snapshot()
		snap.QueuePos = p.queuePosUnsafe(j)
		result = append(result, snap)
	}
	return result
}

func (p *Pool) Cancel(id string) error {
	p.mu.RLock()
	job, ok := p.jobs[id]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("job not found: %s", id)
	}

	status := job.GetStatus()
	if status == StatusSuccess || status == StatusFailed || status == StatusCancelled {
		return fmt.Errorf("job already in terminal state: %s", status)
	}

	job.Cancel()

	if status == StatusQueued || status == StatusPending {
		job.SetStatus(StatusCancelled)
	}

	return nil
}

func (p *Pool) Stop() {
	close(p.stopCh)

	p.mu.Lock()
	for _, job := range p.jobs {
		status := job.GetStatus()
		if status == StatusRunning || status == StatusQueued || status == StatusPending {
			job.Cancel()
		}
	}
	p.mu.Unlock()

	close(p.jobCh)

	shutdownDone := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(30 * time.Second):
		log.Printf("[helixblast] Shutdown timeout: forcing exit (some BLAST jobs may still be running)")
	}

	close(p.doneCh)
}

func (p *Pool) queuePosUnsafe(job *Job) int {
	status := job.GetStatus()
	if status == StatusQueued {
		pos := 0
		for _, j := range p.jobs {
			if j.GetStatus() == StatusQueued && j.CreatedAt.Before(job.CreatedAt) {
				pos++
			}
		}
		return pos + 1
	}
	return 0
}

func (p *Pool) updateQueuePositions() {
	p.mu.Lock()
	defer p.mu.Unlock()

	queued := make([]*Job, 0)
	for _, j := range p.jobs {
		if j.GetStatus() == StatusQueued {
			queued = append(queued, j)
		}
	}

	for i, j := range queued {
		j.QueuePos = i + 1
	}
}

func (p *Pool) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		job, ok := <-p.jobCh
		if !ok {
			return
		}

		if job.IsCancelling() {
			job.SetStatus(StatusCancelled)
			p.updateQueuePositions()
			continue
		}

		job.SetStatus(StatusRunning)
		job.QueuePos = 0
		job.SetProgress("BLAST search in progress")
		p.updateQueuePositions()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		job.SetCancel(cancel)

		hits, err := p.execFn(ctx, job)
		cancel()

		if err != nil {
			log.Printf("[helixblast] Job %s failed: %v", job.ID, err)
			job.SetStatus(StatusFailed)
			job.SetError(err.Error())
			p.updateQueuePositions()
			continue
		}

		if job.IsCancelling() {
			job.SetStatus(StatusCancelled)
			job.SetProgress("")
			p.updateQueuePositions()
			continue
		}

		result := blast.BlastResult{
			JobID:    job.ID,
			Status:   "success",
			Database: job.Database,
			Program:  job.Program,
			Results:  hits,
		}

		data, err := json.Marshal(result)
		if err != nil {
			job.SetStatus(StatusFailed)
			job.SetError(fmt.Sprintf("marshal result: %v", err))
			p.updateQueuePositions()
			continue
		}

		job.SetStatus(StatusSuccess)
		job.SetProgress("")
		job.SetResult(json.RawMessage(data))
		p.updateQueuePositions()
	}
}
