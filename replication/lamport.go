package replication

import (
	"sync/atomic"
)

// LamportClock provides logical clock for event ordering
type LamportClock interface {
	Time() uint64           // Returns current clock value
	Increment() uint64      // Increments and returns new value
	Witness(uint64) uint64  // Updates clock based on received timestamp
}

// MemClock implements Lamport clock with atomic operations
// Based on HashiCorp Serf pattern
type MemClock struct {
	counter uint64
}

// NewMemClock creates a new Lamport clock
func NewMemClock() *MemClock {
	return &MemClock{counter: 0}
}

// Time returns the current clock value
func (c *MemClock) Time() uint64 {
	return atomic.LoadUint64(&c.counter)
}

// Increment increments the clock and returns the new value
func (c *MemClock) Increment() uint64 {
	return atomic.AddUint64(&c.counter, 1)
}

// Witness updates the clock based on a received timestamp
// Algorithm: max(local, received) + 1
func (c *MemClock) Witness(v uint64) uint64 {
	for {
		cur := atomic.LoadUint64(&c.counter)
		next := v
		if cur > v {
			next = cur
		}
		next++

		if atomic.CompareAndSwapUint64(&c.counter, cur, next) {
			return next
		}
		// Retry on CAS failure
	}
}
