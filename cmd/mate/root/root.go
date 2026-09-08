package root

import (
	"github.com/henriquemarlon/mate/cmd/mate/root/review"
	"github.com/henriquemarlon/mate/cmd/mate/root/run"
	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/infra/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const serviceName = "mate"

var Cmd = &cobra.Command{
	Use:     serviceName,
	Short:   "Mate - Turn GoodNotes PDFs into study material",
	Long:    `Mate detects local GoodNotes PDFs, transcribes new handwritten pages with an LLM, and writes auditable study artifacts.`,
	Version: version.BuildVersion,
}

func init() {
	configs.SetDefaults()

	// Configuration every subcommand needs is declared once here. Binding the
	// same key from two commands would leave the running command's flag
	// shadowed by the other command's unchanged default, and silently so.
	// Flag defaults come from the configuration so --help shows real values.
	flags := Cmd.PersistentFlags()
	flags.String("study-dir", viper.GetString(configs.STUDY_DIR), "Local folder containing synced GoodNotes PDFs")
	flags.String("output-dir", viper.GetString(configs.OUTPUT_DIR), "Directory for transcripts, study artifacts, and review images")
	flags.String("state-db", viper.GetString(configs.STATE_DB), "SQLite state database path")
	flags.String("llm-model", viper.GetString(configs.LLM_MODEL), "Chat model identifier requested from the endpoint")
	flags.String("llm-base-url", viper.GetString(configs.LLM_BASE_URL), "OpenAI-compatible chat completion endpoint")
	flags.String("anki-endpoint", viper.GetString(configs.ANKI_ENDPOINT), "AnkiConnect HTTP endpoint")
	flags.String("anki-deck", viper.GetString(configs.ANKI_DECK), "Root Anki deck name")
	flags.Int("dpi", viper.GetInt(configs.DPI), "PDF render DPI (72-600)")
	flags.String("log-level", viper.GetString(configs.LOG_LEVEL), "Log level: debug, info, warn, or error")
	flags.Bool("log-color", viper.GetBool(configs.LOG_COLOR), "Enable colored log output")
	cobra.CheckErr(viper.BindPFlag(configs.STUDY_DIR, flags.Lookup("study-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.OUTPUT_DIR, flags.Lookup("output-dir")))
	cobra.CheckErr(viper.BindPFlag(configs.STATE_DB, flags.Lookup("state-db")))
	cobra.CheckErr(viper.BindPFlag(configs.LLM_MODEL, flags.Lookup("llm-model")))
	cobra.CheckErr(viper.BindPFlag(configs.LLM_BASE_URL, flags.Lookup("llm-base-url")))
	cobra.CheckErr(viper.BindPFlag(configs.ANKI_ENDPOINT, flags.Lookup("anki-endpoint")))
	cobra.CheckErr(viper.BindPFlag(configs.ANKI_DECK, flags.Lookup("anki-deck")))
	cobra.CheckErr(viper.BindPFlag(configs.DPI, flags.Lookup("dpi")))
	cobra.CheckErr(viper.BindPFlag(configs.LOG_LEVEL, flags.Lookup("log-level")))
	cobra.CheckErr(viper.BindPFlag(configs.LOG_COLOR, flags.Lookup("log-color")))

	Cmd.AddCommand(run.Cmd, review.Cmd)
	Cmd.DisableAutoGenTag = true
}
