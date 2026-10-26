# go-skip-list

A concurrent-safe, generic skip list in Go — an ordered map with
O(log n) expected insert, lookup and delete, and sorted iteration.

## The problem

Ordered maps come up constantly: leaderboards, time-series indexes,
in-memory sorted sets. A hash map gives you none of the ordering; a
balanced tree gives you all of it but with rotation logic that is easy to
get wrong. A skip list gets the same asymptotic behaviour with a much
simpler structure — each element sits in a tower of linked levels, and a
search just descends level by level. Redis sorted sets and LevelDB
memtables use exactly this idea.

## Design decisions

1. **Generics over `cmp.Ordered`, no dependencies.** Keys are any ordered
   type (ints, floats, strings); values are arbitrary. The standard
   library's `cmp` package supplies the ordering, so there is nothing to
   install.
2. **One `sync.RWMutex` for the whole list.** Reads run concurrently;
   writes exclude each other. Fine-grained per-node locking would be
   faster under heavy contention but far harder to get right — a single
   lock keeps the code auditable, and the benchmarks below show the
   single-lock cost is small for typical use.
3. **Geometric level distribution with p = 1/4, max level 32.** Each
   tower gains another level with probability 1/4, which is what keeps
   search logarithmic in expectation. Level 32 is unreachable in
   practice (expected once per ~4 billion elements).
4. **Seedable randomness.** `NewWithSeed` makes tower heights
   reproducible, so tests and demos behave identically on every run.
5. **Range queries walk the bottom level only.** Once the first key
   `>= from` is found, the scan is a plain linked-list walk until `to`
   — no extra allocation, no sorting step.

## API

```go
s := skiplist.New[int, string]()
s.Insert(3, "three")        // true when the key is new
v, ok := s.Get(3)           // "three", true
s.Contains(3)              // true
s.Delete(3)                // true when the key was present
s.Len()                    // number of entries
s.All()                    // []Pair, ascending key order
s.Range(2, 5)               // entries with 2 <= key <= 5
min, ok := s.Min()         // smallest entry
max, ok := s.Max()         // largest entry
```

## Run the demo

```bash
go run ./cmd/skldemo
```

It builds a small leaderboard, prints it in score order, and runs a
range query for scores between 600 and 1100.

## Run the tests

```bash
go test ./...
go test -race ./...   # concurrency test included
go test -bench=. ./skiplist
```

## Project structure

```
go.mod                  module definition, no dependencies
skiplist/
  skiplist.go           the list: insert, get, delete, range, min/max
  skiplist_test.go      unit, model-comparison, race and benchmark tests
cmd/skldemo/
  main.go               leaderboard demo
```

## Tech stack

Go 1.23, standard library only (`cmp`, `sync`, `math/rand`).
