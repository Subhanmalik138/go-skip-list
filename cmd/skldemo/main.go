// Command skldemo shows the skip list in action: it inserts a handful of
// leaderboard scores, prints them in order, and runs a range query.
package main

import (
	"fmt"

	"github.com/Subhanmalik138/go-skip-list/skiplist"
)

func main() {
	scores := skiplist.NewWithSeed[int, string](42)
	for _, e := range []struct {
		score int
		name  string
	}{
		{850, "ana"}, {1200, "ben"}, {640, "cy"}, {1250, "dee"},
		{970, "eli"}, {430, "fay"}, {1100, "gus"},
	} {
		scores.Insert(e.score, e.name)
	}

	fmt.Println("Leaderboard (ascending score):")
	for _, p := range scores.All() {
		fmt.Printf("  %4d  %s\n", p.Key, p.Value)
	}

	min, _ := scores.Min()
	max, _ := scores.Max()
	fmt.Printf("\nLowest: %d (%s)  |  Highest: %d (%s)\n",
		min.Key, min.Value, max.Key, max.Value)

	fmt.Println("\nScores between 600 and 1100:")
	for _, p := range scores.Range(600, 1100) {
		fmt.Printf("  %4d  %s\n", p.Key, p.Value)
	}
}
