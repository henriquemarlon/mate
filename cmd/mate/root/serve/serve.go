package serve

import (
	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/infra/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	port       string
	apiKey     string
	apiKeyFile string
	cfg        *configs.MateConfig
)

var Cmd = &cobra.Command{
	Use:     "serve [--port PORT]",
	Short:   "Start the Mate HTTP API server for semantic search",
	Long:    `Starts an HTTP server that exposes a REST API for querying transcribed notes by semantic similarity.`,
	Example: examples,
	Args:    cobra.NoArgs,
	RunE:    run,
	Version: version.BuildVersion,
}

const examples = `# Start HTTP API server:
mate serve

# Start on a specific port:
mate serve --port 3000`

func init() {
	Cmd.Flags().StringVar(&port, "port", "", "HTTP server port (default: 8081)")
	cobra.CheckErr(viper.BindPFlag(configs.HTTP_ADDRESS, Cmd.Flags().Lookup("port")))

	Cmd.Flags().StringVar(&apiKey, "api-key", "", "API key for HTTP API authentication")
	cobra.CheckErr(viper.BindPFlag(configs.API_KEY, Cmd.Flags().Lookup("api-key")))

	Cmd.Flags().StringVar(&apiKeyFile, "api-key-file", "", "Path to file containing API key")
	cobra.CheckErr(viper.BindPFlag(configs.API_KEY_FILE, Cmd.Flags().Lookup("api-key-file")))

	Cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = configs.LoadMateConfig()
		if err != nil {
			return err
		}
		return nil
	}
}

func run(cmd *cobra.Command, args []string) error {
	// TODO: implement HTTP server
	// 1. Set up Qdrant client
	// 2. Set up embeddings client
	// 3. Wire up search handler
	// 4. Start HTTP server
	return nil
}
