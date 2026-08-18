package run

import (
	"errors"
	"time"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/infra/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfg *configs.MateConfig

var Cmd = &cobra.Command{
	Use:   "run",
	Short: "Process new pages from local GoodNotes PDFs",
	Args:  cobra.NoArgs,
	RunE:  run,
}

func init() {
	configs.SetDefaults()
	Cmd.Flags().String("study-dir", "", "Local folder containing synced GoodNotes PDFs")
	Cmd.Flags().String("output-dir", "", "Directory for transcripts and generated study artifacts")
	Cmd.Flags().String("state-db", "", "SQLite state database path")
	Cmd.Flags().String("codex-bin", "", "Path or executable name for the official Codex CLI")
	Cmd.Flags().Int("dpi", 0, "PDF render DPI (72-600)")
	Cmd.Flags().Int("poll-interval", 0, "Interval in seconds between runs; 0 runs once and exits")
	Cmd.Flags().String("log-level", "", "Log level: debug, info, warn, or error")
	Cmd.Flags().Bool("log-color", true, "Enable colored log output")
	cobra.CheckErr(viper.BindPFlag(configs.STUDY_DIR, Cmd.Flags().Lookup("study-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.OUTPUT_DIR, Cmd.Flags().Lookup("output-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.STATE_DB, Cmd.Flags().Lookup("state-db")))
	cobra.CheckErr(viper.BindPFlag(configs.CODEX_BIN, Cmd.Flags().Lookup("codex-bin")))
	cobra.CheckErr(viper.BindPFlag(configs.DPI, Cmd.Flags().Lookup("dpi")))
	cobra.CheckErr(viper.BindPFlag(configs.POLL_INTERVAL_SECONDS, Cmd.Flags().Lookup("poll-interval")))
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

func run(cmd *cobra.Command, _ []string) (err error) {
	mate, err := service.New(cmd.Context(), *cfg)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, mate.Close())
	}()

	ctx := cmd.Context()
	for {
		if _, err := mate.Run(ctx); err != nil {
			return err
		}
		if cfg.PollIntervalSeconds <= 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(cfg.PollIntervalSeconds):
		}
	}
}
