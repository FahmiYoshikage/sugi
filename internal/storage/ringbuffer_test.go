package storage

import (
	"sync"
	"testing"
	"time"

	"github.com/FahmiYoshikage/sugi/internal/model"
)

func makeSnapshot(val float64) model.SystemSnapshot {
	return model.SystemSnapshot{
		Timestamp: time.Now(),
		CPU: model.CPUStats{
			TotalUsage: val,
		},
	}
}

func TestRingBuffer_Basic(t *testing.T) {
	rb := NewRingBuffer(5)

	if rb.Size() != 0 {
		t.Errorf("expected size 0, got %d", rb.Size())
	}
	if rb.Capacity() != 5 {
		t.Errorf("expected capacity 5, got %d", rb.Capacity())
	}

	_, ok := rb.GetLatest()
	if ok {
		t.Error("expected false for empty buffer GetLatest")
	}

	rb.Push(makeSnapshot(10.0))
	if rb.Size() != 1 {
		t.Errorf("expected size 1, got %d", rb.Size())
	}

	latest, ok := rb.GetLatest()
	if !ok || latest.CPU.TotalUsage != 10.0 {
		t.Errorf("expected latest 10.0, got %+v", latest)
	}
}

func TestRingBuffer_WrapAround(t *testing.T) {
	rb := NewRingBuffer(3)

	// Push 1, 2, 3
	rb.Push(makeSnapshot(1.0))
	rb.Push(makeSnapshot(2.0))
	rb.Push(makeSnapshot(3.0))

	all := rb.GetAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 items, got %d", len(all))
	}
	for i, expected := range []float64{1.0, 2.0, 3.0} {
		if all[i].CPU.TotalUsage != expected {
			t.Errorf("all[%d]: expected %f, got %f", i, expected, all[i].CPU.TotalUsage)
		}
	}

	// Push 4 (should overwrite 1.0)
	rb.Push(makeSnapshot(4.0))
	if rb.Size() != 3 {
		t.Errorf("expected size 3 after overwrite, got %d", rb.Size())
	}

	all = rb.GetAll()
	for i, expected := range []float64{2.0, 3.0, 4.0} {
		if all[i].CPU.TotalUsage != expected {
			t.Errorf("after wrap all[%d]: expected %f, got %f", i, expected, all[i].CPU.TotalUsage)
		}
	}

	latest, ok := rb.GetLatest()
	if !ok || latest.CPU.TotalUsage != 4.0 {
		t.Errorf("expected latest 4.0, got %+v", latest)
	}

	// Push 5 (should overwrite 2.0)
	rb.Push(makeSnapshot(5.0))
	all = rb.GetAll()
	for i, expected := range []float64{3.0, 4.0, 5.0} {
		if all[i].CPU.TotalUsage != expected {
			t.Errorf("after 2nd wrap all[%d]: expected %f, got %f", i, expected, all[i].CPU.TotalUsage)
		}
	}
}

func TestRingBuffer_GetLastN(t *testing.T) {
	rb := NewRingBuffer(5)
	for i := 1; i <= 8; i++ {
		rb.Push(makeSnapshot(float64(i)))
	}
	// Buffer has capacity 5, contents are: 4, 5, 6, 7, 8

	last2 := rb.GetLastN(2)
	if len(last2) != 2 || last2[0].CPU.TotalUsage != 7.0 || last2[1].CPU.TotalUsage != 8.0 {
		t.Errorf("unexpected last2: %+v", last2)
	}

	lastAll := rb.GetLastN(10)
	if len(lastAll) != 5 {
		t.Errorf("expected 5 items when n > size, got %d", len(lastAll))
	}
	for i, expected := range []float64{4.0, 5.0, 6.0, 7.0, 8.0} {
		if lastAll[i].CPU.TotalUsage != expected {
			t.Errorf("lastAll[%d]: expected %f, got %f", i, expected, lastAll[i].CPU.TotalUsage)
		}
	}
}

func TestRingBuffer_Concurrent(t *testing.T) {
	rb := NewRingBuffer(100)
	var wg sync.WaitGroup

	// 5 writers
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				rb.Push(makeSnapshot(float64(id*1000 + i)))
			}
		}(w)
	}

	// 5 readers
	for r := 0; r < 5; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = rb.GetAll()
				_ = rb.GetLastN(10)
				_, _ = rb.GetLatest()
			}
		}()
	}

	wg.Wait()
	if rb.Size() != 100 {
		t.Errorf("expected full buffer (100), got %d", rb.Size())
	}
}

func BenchmarkRingBuffer_Push(b *testing.B) {
	rb := NewRingBuffer(3600)
	snap := makeSnapshot(25.5)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rb.Push(snap)
	}
}
