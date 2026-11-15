package app

import (
	"context"
	"log/slog"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
)

type EventDispatcher interface {
	Dispatch(ctx context.Context, ev event.Event) error
}

func NewEventBusCommandMiddleware(dispatcher EventDispatcher, logger *slog.Logger) cqs.CommandHandlerMiddleware {
	return func(next cqs.CommandHandler) cqs.CommandHandler {
		return cqs.CommandHandlerFunc(func(ctx context.Context, cmd cqs.Command) ([]event.Event, error) {
			events, err := next.Handle(ctx, cmd)
			if err != nil {
				return events, err
			}
			for _, ev := range events {
				if dispatchErr := dispatcher.Dispatch(ctx, ev); dispatchErr != nil {
					logger.WarnContext(ctx, "event listeners failed",
						slog.String("command", cmd.Name()),
						slog.String("event", ev.EventName()),
						slog.Any("error", dispatchErr),
					)
				}
			}
			return events, nil
		})
	}
}
