package owsf

import (
	"sort"

	"hop.top/wsm/pkg/model"
)

func CanonicalOrder(events []model.Event) []model.Event {
	if len(events) == 0 {
		return nil
	}

	// 1. Build adjacency list for causal dependencies.
	adj := make(map[string][]string)
	inDegree := make(map[string]int)
	eventMap := make(map[string]model.Event)

	for _, e := range events {
		eventMap[e.ID] = e
		if _, ok := inDegree[e.ID]; !ok {
			inDegree[e.ID] = 0
		}
		for _, dep := range e.CausalDeps {
			adj[dep] = append(adj[dep], e.ID)
			inDegree[e.ID]++
		}
	}

	// 2. Initial queue for events with no incoming dependencies.
	var queue []model.Event
	for _, e := range events {
		if inDegree[e.ID] == 0 {
			queue = append(queue, e)
		}
	}

	var result []model.Event
	for len(queue) > 0 {
		// Rule 1: Respect causal dependencies.
		// Among events ready to be emitted (no pending dependencies),
		// we use the tie-breaking rules: Lamport, WriterID, Seq, ID.
		sort.Slice(queue, func(i, j int) bool {
			a, b := queue[i], queue[j]
			if a.LamportClock != b.LamportClock {
				return a.LamportClock < b.LamportClock
			}
			if a.WriterID != b.WriterID {
				return a.WriterID < b.WriterID
			}
			if a.WriterSeq != b.WriterSeq {
				return a.WriterSeq < b.WriterSeq
			}
			return a.ID < b.ID
		})

		// Pop the "best" candidate.
		curr := queue[0]
		queue = queue[1:]
		result = append(result, curr)

		// Update in-degrees for dependent events.
		for _, nextID := range adj[curr.ID] {
			inDegree[nextID]--
			if inDegree[nextID] == 0 {
				if nextEvent, ok := eventMap[nextID]; ok {
					queue = append(queue, nextEvent)
				}
			}
		}
	}

	// If we didn't process all events, there was a cycle.
	// OWSF documents should not have cycles, but we should handle it
	// by appending remaining events sorted by canonical rules.
	if len(result) < len(events) {
		processed := make(map[string]bool)
		for _, e := range result {
			processed[e.ID] = true
		}
		var remaining []model.Event
		for _, e := range events {
			if !processed[e.ID] {
				remaining = append(remaining, e)
			}
		}
		sort.Slice(remaining, func(i, j int) bool {
			a, b := remaining[i], remaining[j]
			if a.LamportClock != b.LamportClock {
				return a.LamportClock < b.LamportClock
			}
			if a.WriterID != b.WriterID {
				return a.WriterID < b.WriterID
			}
			if a.WriterSeq != b.WriterSeq {
				return a.WriterSeq < b.WriterSeq
			}
			return a.ID < b.ID
		})
		result = append(result, remaining...)
	}

	return result
}
