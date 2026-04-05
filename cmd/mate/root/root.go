package root

import (
	syncCmd "github.com/henriquemarlon/mate/cmd/mate/root/sync"
	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/infra/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const serviceName = "mate"

var Cmd = &cobra.Command{
	Use:     serviceName,
	Short:   "Mate - Transcribe and search handwritten notes",
	Long:    `Mate reads handwritten notes from Google Drive (exported by Goodnotes), transcribes them using a vision model, and builds semantic relations between content using embeddings.`,
	Version: version.BuildVersion,
}

func init() {
	configs.SetDefaults()

	Cmd.PersistentFlags().String("anthropic-api-key", "", "Anthropic API key for vision and embeddings")
	cobra.CheckErr(viper.BindPFlag(configs.ANTHROPIC_API_KEY, Cmd.PersistentFlags().Lookup("anthropic-api-key")))

	Cmd.PersistentFlags().String("anthropic-api-key-file", "", "Path to file containing Anthropic API key")
	cobra.CheckErr(viper.BindPFlag(configs.ANTHROPIC_API_KEY_FILE, Cmd.PersistentFlags().Lookup("anthropic-api-key-file")))

	Cmd.PersistentFlags().String("google-api-key", "", "Google API key for embeddings")
	cobra.CheckErr(viper.BindPFlag(configs.GOOGLE_API_KEY, Cmd.PersistentFlags().Lookup("google-api-key")))

	Cmd.PersistentFlags().String("google-api-key-file", "", "Path to file containing Google API key")
	cobra.CheckErr(viper.BindPFlag(configs.GOOGLE_API_KEY_FILE, Cmd.PersistentFlags().Lookup("google-api-key-file")))

	Cmd.PersistentFlags().String("log-level", "info", "Log level: debug, info, warn or error")
	cobra.CheckErr(viper.BindPFlag(configs.LOG_LEVEL, Cmd.PersistentFlags().Lookup("log-level")))

	Cmd.AddCommand(syncCmd.Cmd)

	Cmd.DisableAutoGenTag = true
}
