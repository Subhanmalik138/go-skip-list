package skiplist

import (
	"fmt"
	"sort"
	"sync"
	"testing"
)

func TestInsertGetAndOverwrite(t *testing.T) {
	s := NewWithSeed[int, string](42)

	if !s.Insert(3, "three") {
		t.Fatal("first insert of a key should report new")
	}
	if s.Insert(3, "THREE") {
		t.Fatal("re-insert of an existing key should report overwrite")
	}
	v, ok := s.Get(3)
	if !ok || v != "THREE" {
		t.Fatalf("Get(3) = %q, %v; want THREE, true", v, ok)
	}
	if _, ok := s.Get(99); ok {
		t.Fatal("Get of a missing key should report false")
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d; want 1 (overwrite must not grow the list)", s.Len())
	}
}

func TestSortedOrderAgainstModel(t *testing.T) {
	s := NewWithSeed[int, int](7)
	model := map[int]int{}
	keys := []int{50, 10, 30, 20, 40, 5, 60, 15, 25, 35}

	for _, k := range keys {
		s.Insert(k, k*10)
		model[k] = k * 10
	}
	if s.Len() != len(keys) {
		t.Fatalf("Len = %d; want %d", s.Len(), len(keys))
	}

	got := s.All()
	if len(got) != len(keys) {
		t.Fatalf("All returned %d entries; want %d", len(got), len(keys))
	}
	sorted := append([]int(nil), keys...)
	sort.Ints(sorted)
	for i, k := range sorted {
		if got[i].Key != k || got[i].Value != model[k] {
			t.Fatalf("All()[%d] = %+v; want key %d value %d", i, got[i], k, model[k])
		}
	}
}

func TestDelete(t *testing.T) {
	s := NewWithSeed[int, string](1)
	for _, k := range []int{1, 2, 3, 4, 5} {
		s.Insert(k, fmt.Sprint(k))
	}
	if s.Delete(99) {
		t.Fatal("Delete of a missing key should report false")
	}
	if !s.Delete(3) {
		t.Fatal("Delete of a present key should report true")
	}
	if s.Contains(3) || s.Len() != 4 {
		t.Fatal("deleted key must be gone and Len must drop")
	}
	// Deleting the head and tail must not break iteration.
	if !s.Delete(1) || !s.Delete(5) {
		t.Fatal("deleting head/tail should succeed")
	}
	got := s.All()
	if len(got) != 2 || got[0].Key != 2 || got[1].Key != 4 {
		t.Fatalf("remaining keys = %v; want [2 4]", got)
	}
}

func TestRangeMinMax(t *testing.T) {
	s := NewWithSeed[int, string](3)
	for i := 1; i <= 10; i++ {
		s.Insert(i, fmt.Sprint(i))
	}

	r := s.Range(3, 6)
	if len(r) != 4 {
		t.Fatalf("Range(3,6) returned %d entries; want 4", len(r))
	}
	for i, p := range r {
		if p.Key != 3+i {
			t.Fatalf("Range(3,6)[%d].Key = %d; want %d", i, p.Key, 3+i)
		}
	}
	if got := s.Range(20, 30); len(got) != 0 {
		t.Fatalf("Range outside the keys should be empty, got %v", got)
	}
	if got := s.Range(6, 3); len(got) != 0 {
		t.Fatalf("inverted Range should be empty, got %v", got)
	}

	min, ok := s.Min()
	if !ok || min.Key != 1 {
		t.Fatalf("Min = %+v, %v; want key 1, true", min, ok)
	}
	max, ok := s.Max()
	if !ok || max.Key != 10 {
		t.Fatalf("Max = %+v, %v; want key 10, true", max, ok)
	}

	empty := New[int, int]()
	if _, ok := empty.Min(); ok {
		t.Fatal("Min on an empty list should report false")
	}
	if _, ok := empty.Max(); ok {
		t.Fatal("Max on an empty list should report false")
	}
}

func TestStringKeys(t *testing.T) {
	s := New[string, int]()
	for k, v := range map[string]int{"banana": 2, "apple": 1, "cherry": 3} {
		s.Insert(k, v)
	}
	got := s.All()
	want := []string{"apple", "banana", "cherry"}
	for i, k := range want {
		if got[i].Key != k {
			t.Fatalf("All()[%d].Key = %q; want %q", i, got[i].Key, k)
		}
	}
}

// TestRandomAgainstSortedModel inserts and deletes a few thousand keys
// in shuffled order and checks the final list against a sorted model —
// the property that catches broken level wiring.
func TestRandomAgainstSortedModel(t *testing.T) {
	s := NewWithSeed[int, int](99)
	model := map[int]int{}
	const n = 5000
	for i := 0; i < n; i++ {
		k := (i * 7919) % 10007 // deterministic pseudo-shuffle
		s.Insert(k, k)
		model[k] = k
	}
	for k := range model {
		if k%3 == 0 {
			s.Delete(k)
			delete(model, k)
		}
	}
	got := s.All()
	if len(got) != len(model) {
		t.Fatalf("All returned %d entries; model has %d", len(got), len(model))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Key >= got[i].Key {
			t.Fatalf("order violated at %d: %d >= %d", i, got[i-1].Key, got[i].Key)
		}
	}
	for _, p := range got {
		if model[p.Key] != p.Value {
			t.Fatalf("entry %+v disagrees with model", p)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New[int, int]()
	const workers = 8
	const perWorker = 500
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				k := w*perWorker + i
				s.Insert(k, k)
				s.Get(k)
				if i%2 == 0 {
					s.Delete(k)
				}
			}
		}(w)
	}
	wg.Wait()
	// Every even key was deleted by its own worker; odd keys remain.
	if s.Len() != workers*perWorker/2 {
		t.Fatalf("Len = %d; want %d", s.Len(), workers*perWorker/2)
	}
	for _, p := range s.All() {
		if p.Key%2 == 0 {
			t.Fatalf("even key %d should have been deleted", p.Key)
		}
	}
}

func BenchmarkInsert(b *testing.B) {
	s := New[int, int]()
	for i := 0; i < b.N; i++ {
		s.Insert(i, i)
	}
}

func BenchmarkGet(b *testing.B) {
	s := New[int, int]()
	for i := 0; i < 10000; i++ {
		s.Insert(i, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Get(i % 10000)
	}
}
