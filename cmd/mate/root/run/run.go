package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/infra/anki"
	"github.com/henriquemarlon/mate/internal/infra/repository/sqlite"
	"github.com/henriquemarlon/mate/internal/service"
	"github.com/henriquemarlon/mate/pkg/codex"
	pkgservice "github.com/henriquemarlon/mate/pkg/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfg *configs.MateConfig

var Cmd = &cobra.Command{
	Use:   "run",
	Short: "Watch local GoodNotes PDFs and process new pages",
	Args:  cobra.NoArgs,
	RunE:  run,
}

func init() {
	configs.SetDefaults()
	Cmd.Flags().String("study-dir", "", "Local folder containing synced GoodNotes PDFs")
	Cmd.Flags().String("output-dir", "", "Directory for transcripts and generated study artifacts")
	Cmd.Flags().String("state-db", "", "SQLite state database path")
	Cmd.Flags().String("codex-bin", "", "Path or executable name for the official Codex CLI")
	Cmd.Flags().String("anki-endpoint", "", "AnkiConnect HTTP endpoint")
	Cmd.Flags().String("anki-deck", "", "Root Anki deck name")
	Cmd.Flags().Int("dpi", 0, "PDF render DPI (72-600)")
	Cmd.Flags().Int("poll-interval", 0, "Interval in seconds between study directory scans")
	Cmd.Flags().String("log-level", "", "Log level: debug, info, warn, or error")
	Cmd.Flags().Bool("notifications", true, "Send a macOS notification when a page needs review")
	Cmd.Flags().Bool("log-color", true, "Enable colored log output")
	cobra.CheckErr(viper.BindPFlag(configs.STUDY_DIR, Cmd.Flags().Lookup("study-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.OUTPUT_DIR, Cmd.Flags().Lookup("output-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.STATE_DB, Cmd.Flags().Lookup("state-db")))
	cobra.CheckErr(viper.BindPFlag(configs.CODEX_BIN, Cmd.Flags().Lookup("codex-bin")))
	cobra.CheckErr(viper.BindPFlag(configs.ANKI_ENDPOINT, Cmd.Flags().Lookup("anki-endpoint")))
	cobra.CheckErr(viper.BindPFlag(configs.ANKI_DECK, Cmd.Flags().Lookup("anki-deck")))
	cobra.CheckErr(viper.BindPFlag(configs.DPI, Cmd.Flags().Lookup("dpi")))
	cobra.CheckErr(viper.BindPFlag(configs.POLL_INTERVAL_SECONDS, Cmd.Flags().Lookup("poll-interval")))
	cobra.CheckErr(viper.BindPFlag(configs.NOTIFICATIONS, Cmd.Flags().Lookup("notifications")))
	cobra.CheckErr(viper.BindPFlag(configs.LOG_LEVEL, Cmd.Flags().Lookup("log-level")))
	cobra.CheckErr(viper.BindPFlag(configs.LOG_COLOR, Cmd.Flags().Lookup("log-color")))

	Cmd.PreRunE = func(_ *cobra.Command, _ []string) error {
		var err error
		cfg, err = configs.LoadMateConfig()
		if err != nil {
			return err
		}
		return nil
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

	codexClient, err := codex.New(codex.Config{Binary: cfg.CodexBin, Logger: logger})
	if err != nil {
		return fmt.Errorf("resolve codex binary (check MATE_CODEX_BIN or --codex-bin): %w", err)
	}

	ankiClient, err := anki.New(cfg.AnkiEndpoint, cfg.AnkiDeck)
	if err != nil {
		return err
	}

	mate, err := service.Create(ctx, &service.CreateInfo{
		Config:     *cfg,
		Logger:     logger,
		Repository: repo,
		Codex:      codexClient,
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
