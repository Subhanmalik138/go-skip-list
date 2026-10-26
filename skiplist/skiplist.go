// Package skiplist implements a concurrent-safe, generic skip list.
//
// A skip list is a probabilistic ordered data structure: each element is
// stored in a tower of linked levels, so search, insert and delete all run
// in O(log n) expected time while iteration stays in sorted order. It
// gives you most of what a balanced tree gives, with far less code — which
// is exactly why it shows up in real systems (Redis sorted sets, LevelDB
// memtables) and in interviews.
package skiplist

import (
	"cmp"
	"math/rand"
	"sync"
)

const (
	// maxLevel caps tower height. With p = 1/4, a level-32 tower is
	// expected once per ~4 billion elements, so 32 is plenty.
	maxLevel = 32
	// p is the probability that a tower grows one level higher.
	p = 0.25
)

// node is one tower in the list. forward[i] is the next node at level i.
type node[K cmp.Ordered, V any] struct {
	key     K
	value   V
	forward []*node[K, V]
}

// SkipList is an ordered map backed by a skip list. Keys must be ordered
// (cmp.Ordered covers ints, floats and strings); values are arbitrary.
// The zero value is not usable — construct with New.
type SkipList[K cmp.Ordered, V any] struct {
	mu     sync.RWMutex
	header *node[K, V]
	level  int // current highest level in use (1-based)
	length int
	rnd    *rand.Rand
}

// New returns an empty skip list. The random source is seeded from the
// global default, so tower heights vary run to run; tests that need
// deterministic heights use NewWithSeed.
func New[K cmp.Ordered, V any]() *SkipList[K, V] {
	return newWithRand[K, V](rand.New(rand.NewSource(rand.Int63())))
}

// NewWithSeed returns an empty skip list whose level draws come from the
// given seed, so behaviour is reproducible (useful for tests and demos).
func NewWithSeed[K cmp.Ordered, V any](seed int64) *SkipList[K, V] {
	return newWithRand[K, V](rand.New(rand.NewSource(seed)))
}

func newWithRand[K cmp.Ordered, V any](rnd *rand.Rand) *SkipList[K, V] {
	return &SkipList[K, V]{
		header: &node[K, V]{forward: make([]*node[K, V], maxLevel)},
		level:  1,
		rnd:    rnd,
	}
}

// randomLevel draws a tower height: level 1 always, each extra level with
// probability p, capped at maxLevel. The geometric distribution is what
// keeps the expected search cost logarithmic.
func (s *SkipList[K, V]) randomLevel() int {
	lvl := 1
	for lvl < maxLevel && s.rnd.Float64() < p {
		lvl++
	}
	return lvl
}

// search walks down from the top level and returns, for every level, the
// last node whose key is strictly smaller than key. Callers must hold at
// least a read lock; Insert upgrades to a write lock before mutating.
func (s *SkipList[K, V]) search(key K) []*node[K, V] {
	update := make([]*node[K, V], maxLevel)
	x := s.header
	for i := s.level - 1; i >= 0; i-- {
		for x.forward[i] != nil && cmp.Compare(x.forward[i].key, key) < 0 {
			x = x.forward[i]
		}
		update[i] = x
	}
	return update
}

// Insert adds key/value, overwriting the value if the key already exists.
// It returns true when the key is new, false when it overwrote.
func (s *SkipList[K, V]) Insert(key K, value V) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	update := s.search(key)
	if next := update[0].forward[0]; next != nil && cmp.Compare(next.key, key) == 0 {
		next.value = value
		return false
	}

	lvl := s.randomLevel()
	if lvl > s.level {
		for i := s.level; i < lvl; i++ {
			update[i] = s.header
		}
		s.level = lvl
	}
	n := &node[K, V]{key: key, value: value, forward: make([]*node[K, V], lvl)}
	for i := 0; i < lvl; i++ {
		n.forward[i] = update[i].forward[i]
		update[i].forward[i] = n
	}
	s.length++
	return true
}

// Get returns the value for key, and whether the key was present.
func (s *SkipList[K, V]) Get(key K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	update := s.search(key)
	if next := update[0].forward[0]; next != nil && cmp.Compare(next.key, key) == 0 {
		return next.value, true
	}
	var zero V
	return zero, false
}

// Contains reports whether key is present.
func (s *SkipList[K, V]) Contains(key K) bool {
	_, ok := s.Get(key)
	return ok
}

// Delete removes key and reports whether it was present.
func (s *SkipList[K, V]) Delete(key K) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	update := s.search(key)
	target := update[0].forward[0]
	if target == nil || cmp.Compare(target.key, key) != 0 {
		return false
	}
	for i := 0; i < len(target.forward); i++ {
		update[i].forward[i] = target.forward[i]
	}
	// Shrink the top level while it points nowhere; the list never
	// needs levels above its tallest remaining tower.
	for s.level > 1 && s.header.forward[s.level-1] == nil {
		s.level--
	}
	s.length--
	return true
}

// Len returns the number of entries.
func (s *SkipList[K, V]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.length
}

// Pair is one key/value entry returned by iteration.
type Pair[K cmp.Ordered, V any] struct {
	Key   K
	Value V
}

// All returns every entry in ascending key order. The snapshot is taken
// under a read lock, so concurrent writers cannot corrupt the walk.
func (s *SkipList[K, V]) All() []Pair[K, V] {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Pair[K, V], 0, s.length)
	for x := s.header.forward[0]; x != nil; x = x.forward[0] {
		out = append(out, Pair[K, V]{Key: x.key, Value: x.value})
	}
	return out
}

// Range returns entries with from <= key <= to, in ascending order.
// An empty result (not nil) is returned when the interval holds nothing.
func (s *SkipList[K, V]) Range(from, to K) []Pair[K, V] {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := []Pair[K, V]{}
	x := s.header
	// Descend to the first node >= from, same walk as search.
	for i := s.level - 1; i >= 0; i-- {
		for x.forward[i] != nil && cmp.Compare(x.forward[i].key, from) < 0 {
			x = x.forward[i]
		}
	}
	for x = x.forward[0]; x != nil && cmp.Compare(x.key, to) <= 0; x = x.forward[0] {
		out = append(out, Pair[K, V]{Key: x.key, Value: x.value})
	}
	return out
}

// Min returns the smallest entry, and whether the list is non-empty.
func (s *SkipList[K, V]) Min() (Pair[K, V], bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	first := s.header.forward[0]
	if first == nil {
		return Pair[K, V]{}, false
	}
	return Pair[K, V]{Key: first.key, Value: first.value}, true
}

// Max returns the largest entry, and whether the list is non-empty.
func (s *SkipList[K, V]) Max() (Pair[K, V], bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	x := s.header
	for i := s.level - 1; i >= 0; i-- {
		for x.forward[i] != nil {
			x = x.forward[i]
		}
	}
	if x == s.header {
		return Pair[K, V]{}, false
	}
	return Pair[K, V]{Key: x.key, Value: x.value}, true
}
