package run

import (
	"errors"

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
	Cmd.Flags().String("anki-endpoint", "", "AnkiConnect HTTP endpoint")
	Cmd.Flags().String("anki-deck", "", "Root Anki deck name")
	Cmd.Flags().Int("dpi", 0, "PDF render DPI (72-600)")
	Cmd.Flags().String("log-level", "", "Log level: debug, info, warn, or error")
	Cmd.Flags().Bool("log-color", true, "Enable colored log output")
	cobra.CheckErr(viper.BindPFlag(configs.STUDY_DIR, Cmd.Flags().Lookup("study-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.OUTPUT_DIR, Cmd.Flags().Lookup("output-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.STATE_DB, Cmd.Flags().Lookup("state-db")))
	cobra.CheckErr(viper.BindPFlag(configs.CODEX_BIN, Cmd.Flags().Lookup("codex-bin")))
	cobra.CheckErr(viper.BindPFlag(configs.ANKI_ENDPOINT, Cmd.Flags().Lookup("anki-endpoint")))
	cobra.CheckErr(viper.BindPFlag(configs.ANKI_DECK, Cmd.Flags().Lookup("anki-deck")))
	cobra.CheckErr(viper.BindPFlag(configs.DPI, Cmd.Flags().Lookup("dpi")))
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

	_, err = mate.Run(cmd.Context())
	return err
}
