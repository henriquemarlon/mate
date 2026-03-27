package sync

import (
	"context"
	"log/slog"
	"os"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/infra/anki"
	"github.com/henriquemarlon/mate/internal/infra/drive"
	"github.com/henriquemarlon/mate/internal/infra/embeddings"
	"github.com/henriquemarlon/mate/internal/infra/flashcardgen"
	"github.com/henriquemarlon/mate/internal/infra/statedb"
	"github.com/henriquemarlon/mate/internal/infra/store"
	"github.com/henriquemarlon/mate/internal/infra/version"
	"github.com/henriquemarlon/mate/internal/infra/vision"
	"github.com/henriquemarlon/mate/internal/usecase"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	folderId        string
	credentialsFile string
	noAnki          bool
	cfg             *configs.MateConfig
)

var Cmd = &cobra.Command{
	Use:     "sync [--folder-id ID] [--credentials-file FILE]",
	Short:   "Sync and transcribe Goodnotes notebooks from Google Drive",
	Long:    `Downloads Goodnotes exports from Google Drive, transcribes handwritten content using a vision model, generates embeddings, clusters pages via DBSCAN, and auto-generates Anki flashcards per cluster.`,
	Example: examples,
	Args:    cobra.NoArgs,
	RunE:    run,
	Version: version.BuildVersion,
}

const examples = `# Sync all notebooks from a Drive folder:
mate sync --folder-id 1ABC123 --credentials-file ~/.mate/secrets/drive_credentials.json

# Sync using environment variables:
MATE_DRIVE_FOLDER_ID=1ABC123 MATE_DRIVE_CREDENTIALS_FILE=creds.json mate sync

# Sync without pushing to Anki:
mate sync --no-anki`

func init() {
	Cmd.Flags().StringVar(&folderId, "folder-id", "", "Google Drive folder ID containing Goodnotes exports")
	cobra.CheckErr(viper.BindPFlag(configs.DRIVE_FOLDER_ID, Cmd.Flags().Lookup("folder-id")))

	Cmd.Flags().StringVar(&credentialsFile, "credentials-file", "", "Path to Google service account credentials JSON file")
	cobra.CheckErr(viper.BindPFlag(configs.DRIVE_CREDENTIALS_FILE, Cmd.Flags().Lookup("credentials-file")))

	Cmd.Flags().BoolVar(&noAnki, "no-anki", false, "Skip Anki flashcard generation")

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
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	// --- Step 1-3: Drive → Vision → Embed → Qdrant ---

	driveClient, err := drive.NewClient(ctx, cfg.DriveCredentials.Value, cfg.DriveFolderId)
	if err != nil {
		return err
	}

	visionClient := vision.NewClient(cfg.AnthropicApiKey.Value)
	embeddingsClient := embeddings.NewClient(cfg.GoogleApiKey.Value, cfg.GoogleEmbeddingModel)

	storeClient, err := store.NewClient(cfg.QdrantAddress, cfg.QdrantCollection)
	if err != nil {
		return err
	}

	syncUC := usecase.NewSyncNotes(driveClient, visionClient, embeddingsClient, storeClient, logger)
	if _, err := syncUC.Execute(ctx); err != nil {
		return err
	}

	// --- Step 4-5: Cluster → Detect dirty ---

	dbPath, err := configs.ExpandPath(cfg.StateDbPath)
	if err != nil {
		return err
	}

	stateDB, err := statedb.Open(dbPath)
	if err != nil {
		return err
	}
	defer stateDB.Close()

	epsilon, minPts, err := usecase.ParseDBSCANParams(cfg.DbscanEpsilon, cfg.DbscanMinPoints)
	if err != nil {
		return err
	}

	clusterUC := usecase.NewClusterNotes(storeClient, stateDB, epsilon, minPts, logger)
	dirty, stale, err := clusterUC.Execute(ctx)
	if err != nil {
		return err
	}

	if len(dirty) == 0 && len(stale) == 0 {
		logger.Info("no cluster changes detected, nothing to do")
		return nil
	}

	// --- Step 6: Generate flashcards + push to Anki ---

	if noAnki {
		logger.Info("skipping Anki (--no-anki flag set)", "dirty_clusters", len(dirty), "stale_clusters", len(stale))
		return nil
	}

	ankiClient := anki.NewClient(cfg.AnkiConnectUrl, cfg.AnkiDeckPrefix, cfg.AnkiModelName)

	// Check if Anki is reachable
	if err := ankiClient.Ping(ctx); err != nil {
		logger.Warn("Anki is not reachable, skipping flashcard generation", "error", err)
		return nil
	}

	generator := flashcardgen.NewGenerator(cfg.AnthropicApiKey.Value)
	genUC := usecase.NewGenerateFlashcards(generator, ankiClient, storeClient, stateDB, logger)

	// Process dirty clusters
	for _, dc := range dirty {
		if err := genUC.ProcessDirtyCluster(ctx, dc); err != nil {
			logger.Error("failed to process cluster", "cluster_id", dc.ClusterID, "error", err)
			continue
		}
	}

	// Cleanup stale clusters
	for _, sc := range stale {
		if err := genUC.CleanupStaleCluster(ctx, sc); err != nil {
			logger.Error("failed to cleanup stale cluster", "cluster_id", sc.ClusterID, "error", err)
		}
	}

	logger.Info("sync pipeline complete",
		"dirty_clusters", len(dirty),
		"stale_clusters", len(stale),
	)

	return nil
}
