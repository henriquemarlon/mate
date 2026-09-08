package run

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
	"github.com/spf13/viper"
)

var cfg *configs.MateConfig

var Cmd = &cobra.Command{
	Use:   "run",
	Short: "Watch local GoodNotes PDFs and process new pages",
	Long: `Scans the study directory on a timer, transcribes pages that changed,
writes the study artifacts, and synchronizes the cards with Anki. A page the
transcriber cannot confirm is set aside for "mate review" instead of guessed.

Every flag can also be set through its MATE_ environment variable; the
generated reference in docs/config.md lists them with their defaults.`,
	Example: runExamples,
	Args:    cobra.NoArgs,
	RunE:    run,
}

const runExamples = `# Watch the configured study directory:
mate run

# Watch a specific directory and scan every five minutes:
mate run --study-dir ~/GoodNotes --poll-interval 300`

func init() {
	configs.SetDefaults()
	flags := Cmd.Flags()
	flags.Int("poll-interval", viper.GetInt(configs.POLL_INTERVAL_SECONDS), "Interval in seconds between study directory scans")
	flags.Bool("notifications", viper.GetBool(configs.NOTIFICATIONS), "Send a macOS notification when a page needs review")
	cobra.CheckErr(viper.BindPFlag(configs.POLL_INTERVAL_SECONDS, flags.Lookup("poll-interval")))
	cobra.CheckErr(viper.BindPFlag(configs.NOTIFICATIONS, flags.Lookup("notifications")))

	Cmd.PreRunE = func(_ *cobra.Command, _ []string) error {
		var err error
		cfg, err = configs.LoadMateConfig()
		return err
	}
}

// run acquires every external resource, wires them into the Mate service,
// and serves until the context is cancelled. Resource lifetimes belong here.
func run(cmd *cobra.Command, _ []string) (err error) {
	ctx := cmd.Context()
	logger := pkgservice.NewLogger(service.ServiceName, cfg.LogLevel, cfg.LogColor)

	repo, err := sqlite.NewSQLiteRepository(ctx, cfg.StateDB)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, repo.Close())
	}()

	llmClient, err := llm.New(llm.Config{
		APIKey:         cfg.LLMAPIKey.Value,
		Model:          cfg.LLMModel,
		BaseURL:        cfg.LLMBaseURL,
		RequestTimeout: cfg.LLMTimeoutSeconds,
		Logger:         logger,
	})
	if err != nil {
		return fmt.Errorf("configure llm client: %w", err)
	}

	ankiClient, err := anki.New(cfg.AnkiEndpoint, cfg.AnkiDeck)
	if err != nil {
		return err
	}

	mate, err := service.Create(ctx, &service.CreateInfo{
		Config:     *cfg,
		Logger:     logger,
		Repository: repo,
		LLM:        llmClient,
		Anki:       ankiClient,
	})
	if err != nil {
		return err
	}

	err = mate.Serve(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
