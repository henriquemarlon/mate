package root

import (
	"github.com/henriquemarlon/mate/cmd/mate/root/run"
	"github.com/henriquemarlon/mate/internal/infra/version"
	"github.com/spf13/cobra"
)

const serviceName = "mate"

var Cmd = &cobra.Command{
	Use:     serviceName,
	Short:   "Mate - Turn GoodNotes PDFs into study material",
	Long:    `Mate detects local GoodNotes PDFs, transcribes new handwritten pages with an LLM, and writes auditable study artifacts.`,
	Version: version.BuildVersion,
}

func init() {
	Cmd.AddCommand(run.Cmd)
	Cmd.DisableAutoGenTag = true
}
