package app

import (
	"fmt"

	"github.com/bruli-lab/go-core/event"
)

func EventsOf[T event.Event](events []event.Event) []T {
	matching := make([]T, 0, len(events))
	for _, ev := range events {
		if typed, ok := ev.(T); ok {
			matching = append(matching, typed)
		}
	}
	return matching
}

func SingleEvent[T event.Event](events []event.Event) (T, error) {
	matching := EventsOf[T](events)
	if len(matching) != 1 {
		var zero T
		return zero, fmt.Errorf("expected one %T event, got %d of %d events", zero, len(matching), len(events))
	}
	return matching[0], nil
}

func QueryResult[T any](result any, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	typed, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("unexpected query result type: %T", result)
	}
	return typed, nil
}
