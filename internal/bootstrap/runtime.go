package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	observabilityinfra "github.com/bruli-lab/stowmark/internal/infra/observability"
)

var DomainEvents = []string{
	snapshot.SnapshotCreatedEventName,
	snapshot.SnapshotRestoredEventName,
	snapshot.FileRestoredEventName,
	snapshot.SnapshotForgottenEventName,
	snapshot.ObjectsPrunedEventName,
	snapshot.MissingObjectsDetectedEventName,
	snapshot.GenerationsRemovedEventName,
	snapshot.TemporariesRemovedEventName,
	repository.RepositoryInitializedEventName,
	repository.LockRemovedEventName,
	encryption.SymmetricKeyRotatedEventName,
	encryption.KeyRewrappedEventName,
}

type Runtime struct {
	environment   map[string]string
	observability *observabilityinfra.OTLPObservability
	bus           event.Bus
	commands      cqs.CommandHandlerMiddleware
	queries       cqs.QueryHandlerMiddleware
	asymmetric    app.AsymmetricKeyPairRepository
	symmetric     app.SymmetricKeyRepository
	keyGenerator  app.KeyGenerator
	source        app.SourceRepository
}

func NewRuntime(ctx context.Context, environment map[string]string) (*Runtime, error) {
	observabilityConfig, err := config.NewObservabilityConfigFrom(environment)
	if err != nil {
		return nil, fmt.Errorf("read observability configuration: %w", err)
	}
	obsv, err := observabilityinfra.New(ctx, observabilityConfig)
	if err != nil {
		return nil, fmt.Errorf("create observability: %w", err)
	}
	bus, err := newEventBus(obsv)
	if err != nil {
		_ = obsv.Shutdown(ctx)
		return nil, err
	}
	commands, err := middlewares.BuildCommandMiddlewares(obsv, bus)
	if err != nil {
		_ = obsv.Shutdown(ctx)
		return nil, err
	}
	queries, err := middlewares.BuildQueryMiddlewares(obsv)
	if err != nil {
		_ = obsv.Shutdown(ctx)
		return nil, err
	}
	return &Runtime{
		environment:   environment,
		observability: obsv,
		bus:           bus,
		commands:      commands,
		queries:       queries,
		asymmetric:    disk.NewAsymmetricKeyPairRepository(),
		symmetric:     encrypt.NewSymmetricRepository(),
		keyGenerator:  encrypt.NewRSAKeyGenerator(),
		source:        disk.NewSourceRepository(),
	}, nil
}

func newEventBus(obsv *observabilityinfra.OTLPObservability) (event.Bus, error) {
	metrics, err := middlewares.NewEventMetricsListener(obsv.MeterProvider)
	if err != nil {
		return nil, err
	}
	common := []event.Listener{app.NewEventLogger(obsv.Logger), app.NewSpanEventRecorder(), metrics}
	specific := map[string][]event.Listener{
		snapshot.SnapshotCreatedEventName:  {app.NewSnapshotCreatedTracer()},
		snapshot.SnapshotRestoredEventName: {app.NewSnapshotRestoredTracer()},
	}
	bus := event.NewBus()
	for _, name := range DomainEvents {
		listeners := append([]event.Listener{}, common...)
		bus.Subscribe(name, append(listeners, specific[name]...)...)
	}
	return bus, nil
}

func (r *Runtime) Logger() *slog.Logger {
	return r.observability.Logger
}

func (r *Runtime) Listeners(eventName string) []event.Listener {
	return r.bus[eventName]
}

func (r *Runtime) GenerateKey() cqs.CommandHandler {
	return r.commands(app.NewGenerateKey(r.keyGenerator, r.asymmetric))
}

func (r *Runtime) Close(ctx context.Context) error {
	return r.observability.Shutdown(ctx)
}
