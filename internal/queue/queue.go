package queue

import (
	"context"
	"log"
	"sync"

	"linkpulse/internal/store"
)

// Publisher is the edge between the HTTP handler and the click-processing
// pipeline. The local version uses ClickQueue. An SQS publisher can implement
// the same interface later.
type Publisher interface {
	Publish(event store.ClickEvent)
}

// ClickQueue is a tiny in-process event queue. It exists to show the same
// shape as SQS: the redirect handler publishes an event and returns quickly;
// a worker writes the event to storage.
type ClickQueue struct {
	store  store.Store
	events chan store.ClickEvent
	wg     sync.WaitGroup
}

func New(st store.Store, buffer int) *ClickQueue {
	if buffer <= 0 {
		buffer = 1024
	}
	return &ClickQueue{
		store:  st,
		events: make(chan store.ClickEvent, buffer),
	}
}

func (q *ClickQueue) Start(ctx context.Context) {
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-q.events:
				if err := q.store.RecordClick(context.Background(), event); err != nil {
					log.Printf("record click: %v", err)
				}
			}
		}
	}()
}

func (q *ClickQueue) Publish(event store.ClickEvent) {
	select {
	case q.events <- event:
	default:
		// Never block a redirect because analytics is slow. A real system would
		// retry through SQS and use a dead-letter queue for failures.
		log.Printf("click queue full; dropping event for %s", event.Code)
	}
}

func (q *ClickQueue) Wait() {
	q.wg.Wait()
}
