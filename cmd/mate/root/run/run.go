package run

import (
	"errors"
	"log/slog"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/workflow"
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
	cobra.CheckErr(viper.BindPFlag(configs.STUDY_DIR, Cmd.Flags().Lookup("study-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.OUTPUT_DIR, Cmd.Flags().Lookup("output-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.STATE_DB, Cmd.Flags().Lookup("state-db")))
	cobra.CheckErr(viper.BindPFlag(configs.CODEX_BIN, Cmd.Flags().Lookup("codex-bin")))
	cobra.CheckErr(viper.BindPFlag(configs.DPI, Cmd.Flags().Lookup("dpi")))

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
	logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: cfg.LogLevel}))
	runner, err := workflow.New(*cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, runner.Close())
	}()

	logger.Info("configuration",
		"study_dir", cfg.StudyDir,
		"output_dir", cfg.OutputDir,
		"state_db", cfg.StateDB,
		"codex_bin", cfg.CodexBin,
		"dpi", cfg.DPI,
		"log_level", cfg.LogLevel.String(),
	)
	_, err = runner.Run(cmd.Context())
	return err
}
