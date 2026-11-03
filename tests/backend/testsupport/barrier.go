package testsupport

import (
	"context"
	"errors"
	"sync"
)

// Barrier releases every participant once the configured number of callers
// have arrived. A cancelled waiter leaves the barrier available to later
// attempts; callers that were already waiting observe their own context.
type Barrier struct {
	parties int
	mu      sync.Mutex
	arrived int
	release chan struct{}
	once    sync.Once
}

func NewBarrier(parties int) *Barrier {
	return &Barrier{parties: parties, release: make(chan struct{})}
}

func (b *Barrier) Wait(ctx context.Context) error {
	if b == nil || b.parties < 1 {
		return errors.New("barrier parties must be positive")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.Lock()
	select {
	case <-b.release:
		b.mu.Unlock()
		return nil
	default:
	}
	b.arrived++
	if b.arrived >= b.parties {
		b.once.Do(func() { close(b.release) })
	}
	b.mu.Unlock()

	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		b.mu.Lock()
		select {
		case <-b.release:
			b.mu.Unlock()
			return nil
		default:
			b.arrived--
		}
		b.mu.Unlock()
		return ctx.Err()
	}
}
