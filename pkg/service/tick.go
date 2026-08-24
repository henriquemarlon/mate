package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// TickImpl is implemented by services that process work in periodic ticks.
// Returning reschedule=true requests an immediate follow-up tick. An error
// wrapping ErrServiceStopped aborts Serve; any other error is logged and the
// service keeps ticking.
type TickImpl interface {
	Tick(ctx context.Context) (reschedule bool, err error)
}

type TickServiceTemplate struct {
	BaseTemplate
	tickImpl TickImpl
	interval time.Duration
}

type TickServiceConfigs struct {
	BaseConfigs
	PollInterval time.Duration
}

func InitTickServiceTemplate(
	s *TickServiceTemplate,
	cfg *TickServiceConfigs,
	tickImpl TickImpl,
) error {
	if s == nil || cfg == nil || tickImpl == nil {
		return ErrInvalid
	}

	InitServiceTemplate(&s.BaseTemplate, &cfg.BaseConfigs)

	s.tickImpl = tickImpl

	if cfg.PollInterval < 0 {
		return fmt.Errorf("PollInterval must be non-negative, got %v", cfg.PollInterval)
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Minute
	}
	s.interval = cfg.PollInterval

	return nil
}

func (s *TickServiceTemplate) tick(ctx context.Context) error {
	reschedule := true
	for reschedule && ctx.Err() == nil {
		var err error

		start := time.Now()
		reschedule, err = s.tickImpl.Tick(ctx)
		elapsed := time.Since(start)

		switch {
		case errors.Is(err, ErrServiceStopped):
			s.Logger.Error("Tick", "duration", elapsed, "error", err)
			return err
		case errors.Is(err, context.Canceled):
			// Expected during shutdown; not worth an error record.
		case err != nil:
			s.Logger.Error("Tick", "duration", elapsed, "reschedule", reschedule, "error", err)
		default:
			s.Logger.Debug("Tick", "duration", elapsed, "reschedule", reschedule)
		}
	}
	return nil
}

func (s *TickServiceTemplate) Serve(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	if err := s.tick(ctx); err != nil {
		return err
	}
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}
