package app

import (
	"github.com/bruli-lab/go-core/event"
	"go.opentelemetry.io/otel/trace"
)

type EventTracer interface {
	Trace(span trace.Span, ev event.Event)
}
type EventsTracing struct {
	events map[string]EventTracer
}

func (e *EventsTracing) Add(eventName string, tracer EventTracer) {
	e.events[eventName] = tracer
}

func (e *EventsTracing) Trace(span trace.Span, ev event.Event) {
	tracer, ok := e.events[ev.EventName()]
	if !ok {
		return
	}
	tracer.Trace(span, ev)
}

func NewEventsTracing() *EventsTracing {
	return &EventsTracing{events: make(map[string]EventTracer)}
}
