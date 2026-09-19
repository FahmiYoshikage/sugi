package storage

import (
	"sync"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

// DefaultRingBufferCapacity holds 3600 points (1 point/second = 1 hour of time series).
const DefaultRingBufferCapacity = 3600

// RingBuffer is a thread-safe in-memory circular buffer for time-series metric snapshots.
// It provides zero-allocation updates on the hot write path.
type RingBuffer struct {
	mu       sync.RWMutex
	data     []model.SystemSnapshot
	capacity int
	head     int // points to the next write location
	size     int // current number of stored elements
}

// NewRingBuffer creates a new fixed-capacity RingBuffer.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultRingBufferCapacity
	}
	return &RingBuffer{
		data:     make([]model.SystemSnapshot, capacity),
		capacity: capacity,
		head:     0,
		size:     0,
	}
}

// Push adds a new snapshot to the ring buffer. If full, it overwrites the oldest element.
// This operation is zero-allocation.
func (r *RingBuffer) Push(snapshot model.SystemSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = snapshot
	r.head = (r.head + 1) % r.capacity
	if r.size < r.capacity {
		r.size++
	}
}

// GetLatest returns the most recently inserted snapshot.
func (r *RingBuffer) GetLatest() (model.SystemSnapshot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return model.SystemSnapshot{}, false
	}

	idx := (r.head - 1 + r.capacity) % r.capacity
	return r.data[idx], true
}

// GetAll returns all stored snapshots ordered chronologically from oldest to newest.
func (r *RingBuffer) GetAll() []model.SystemSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return []model.SystemSnapshot{}
	}

	result := make([]model.SystemSnapshot, r.size)
	if r.size < r.capacity {
		// Buffer hasn't wrapped around yet; data is in [0, size)
		copy(result, r.data[:r.size])
	} else {
		// Buffer has wrapped around; oldest data is at head
		firstChunk := r.capacity - r.head
		copy(result[:firstChunk], r.data[r.head:])
		copy(result[firstChunk:], r.data[:r.head])
	}

	return result
}

// GetLastN returns the most recent n snapshots ordered chronologically from oldest to newest.
func (r *RingBuffer) GetLastN(n int) []model.SystemSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 || n <= 0 {
		return []model.SystemSnapshot{}
	}

	if n > r.size {
		n = r.size
	}

	result := make([]model.SystemSnapshot, n)
	// Start index of the oldest requested item among the last n
	startIdx := (r.head - n + r.capacity) % r.capacity

	if startIdx+n <= r.capacity {
		// Contiguous segment
		copy(result, r.data[startIdx:startIdx+n])
	} else {
		// Wrapped segment
		firstChunk := r.capacity - startIdx
		copy(result[:firstChunk], r.data[startIdx:])
		copy(result[firstChunk:], r.data[:n-firstChunk])
	}

	return result
}

// Size returns the current number of elements stored in the buffer.
func (r *RingBuffer) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

// Capacity returns the maximum storage capacity of the buffer.
func (r *RingBuffer) Capacity() int {
	return r.capacity
}

// Clear resets the buffer state.
func (r *RingBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.head = 0
	r.size = 0
}
