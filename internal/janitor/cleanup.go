package janitor

import (
	"context"
	"log"
	"time"

	"helixblast/internal/storage"
)

type Janitor struct {
	store       storage.Store
	ttl         time.Duration
	interval    time.Duration
	stopCh      chan struct{}
	doneCh      chan struct{}
}

func New(store storage.Store, ttlHours int) *Janitor {
	return &Janitor{
		store:    store,
		ttl:      time.Duration(ttlHours) * time.Hour,
		interval: 10 * time.Minute,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

func (j *Janitor) Start() {
	go j.loop()
}

func (j *Janitor) Stop() {
	close(j.stopCh)
	<-j.doneCh
}

func (j *Janitor) loop() {
	defer close(j.doneCh)

	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	j.cleanup()

	for {
		select {
		case <-j.stopCh:
			return
		case <-ticker.C:
			j.cleanup()
		}
	}
}

func (j *Janitor) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	expired, err := j.store.ListExpired(ctx, j.ttl)
	if err != nil {
		log.Printf("[helixblast] Janitor: failed to list expired: %v", err)
		return
	}

	if len(expired) == 0 {
		return
	}

	log.Printf("[helixblast] Janitor: cleaning up %d expired job(s)", len(expired))
	for _, key := range expired {
		if err := j.store.Delete(ctx, key); err != nil {
			log.Printf("[helixblast] Janitor: failed to delete %s: %v", key, err)
		}
	}
}
