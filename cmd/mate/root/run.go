package root

import (
	"context"
	"errors"

	"github.com/henriquemarlon/mate/configs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Watch local GoodNotes PDFs and process new pages",
	Long: `Scans the study directory on a timer, transcribes pages that changed,
writes the study artifacts, and synchronizes the cards with Anki. A page the
transcriber cannot confirm is set aside for "mate review" instead of guessed.

Every flag can also be set through its MATE_ environment variable; the
generated reference in docs/config.md lists them with their defaults.`,
	Example: runExamples,
	Args:    cobra.NoArgs,
	PreRunE: loadConfig,
	RunE:    serve,
}

const runExamples = `# Watch the configured study directory:
mate run

# Watch a specific directory and scan every five minutes:
mate run --study-dir ~/GoodNotes --poll-interval 300`

func init() {
	configs.SetDefaults()
	flags := runCmd.Flags()
	flags.Int("poll-interval", viper.GetInt(configs.POLL_INTERVAL_SECONDS), "Interval in seconds between study directory scans")
	flags.Bool("notifications", viper.GetBool(configs.NOTIFICATIONS), "Send a macOS notification when a page needs review")
	cobra.CheckErr(viper.BindPFlag(configs.POLL_INTERVAL_SECONDS, flags.Lookup("poll-interval")))
	cobra.CheckErr(viper.BindPFlag(configs.NOTIFICATIONS, flags.Lookup("notifications")))
}

// serve runs the Mate service until the context is cancelled.
func serve(cmd *cobra.Command, _ []string) (err error) {
	ctx := cmd.Context()
	mate, closeRepo, err := mateService(ctx)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, closeRepo())
	}()

	err = mate.Serve(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
