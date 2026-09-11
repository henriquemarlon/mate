package root

import (
	"context"
	"errors"
	"fmt"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/infra/anki"
	"github.com/henriquemarlon/mate/internal/infra/repository/sqlite"
	"github.com/henriquemarlon/mate/internal/service"
	"github.com/henriquemarlon/mate/pkg/llm"
	pkgservice "github.com/henriquemarlon/mate/pkg/service"
	"github.com/spf13/cobra"
)

var cfg *configs.MateConfig

// loadConfig runs before every command. The persistent flags are already
// parsed by then, so the configuration is read once and shared.
func loadConfig(_ *cobra.Command, _ []string) error {
	var err error
	cfg, err = configs.LoadMateConfig()
	return err
}

// mateService acquires the external resources both commands need and returns
// the service alongside the closer that releases them. The closer belongs to
// the caller so each command can join it with its own error.
func mateService(ctx context.Context) (*service.Service, func() error, error) {
	logger := pkgservice.NewLogger(service.ServiceName, cfg.LogLevel, cfg.LogColor)

	repo, err := sqlite.NewSQLiteRepository(ctx, cfg.StateDB)
	if err != nil {
		return nil, nil, err
	}

	llmClient, err := llm.New(llm.Config{
		APIKey:         cfg.LLMAPIKey.Value,
		Model:          cfg.LLMModel,
		BaseURL:        cfg.LLMBaseURL,
		RequestTimeout: cfg.LLMTimeoutSeconds,
		Logger:         logger,
	})
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("configure llm client: %w", err), repo.Close())
	}

	ankiClient, err := anki.New(cfg.AnkiEndpoint, cfg.AnkiDeck)
	if err != nil {
		return nil, nil, errors.Join(err, repo.Close())
	}

	mate, err := service.Create(ctx, &service.CreateInfo{
		Config:     *cfg,
		Logger:     logger,
		Repository: repo,
		LLM:        llmClient,
		Anki:       ankiClient,
	})
	if err != nil {
		return nil, nil, errors.Join(err, repo.Close())
	}
	return mate, repo.Close, nil
}
