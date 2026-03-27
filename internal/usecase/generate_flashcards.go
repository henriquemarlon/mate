package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/henriquemarlon/mate/internal/infra/anki"
	"github.com/henriquemarlon/mate/internal/infra/flashcardgen"
	"github.com/henriquemarlon/mate/internal/infra/statedb"
	"github.com/henriquemarlon/mate/internal/infra/store"
)

const bridgePageLimit = 5

// GenerateFlashcards orchestrates: label + bridge context + generate + Anki push (pipeline step 6).
type GenerateFlashcards struct {
	generator *flashcardgen.Generator
	anki      *anki.Client
	store     *store.Client
	stateDB   *statedb.DB
	logger    *slog.Logger
}

// NewGenerateFlashcards creates a new GenerateFlashcards usecase.
func NewGenerateFlashcards(
	generator *flashcardgen.Generator,
	ankiClient *anki.Client,
	storeClient *store.Client,
	stateDB *statedb.DB,
	logger *slog.Logger,
) *GenerateFlashcards {
	return &GenerateFlashcards{
		generator: generator,
		anki:      ankiClient,
		store:     storeClient,
		stateDB:   stateDB,
		logger:    logger,
	}
}

// ProcessDirtyCluster generates flashcards for a dirty cluster and pushes them to Anki.
func (g *GenerateFlashcards) ProcessDirtyCluster(ctx context.Context, dc DirtyCluster) error {
	g.logger.Info("generating flashcards for cluster",
		"cluster_id", dc.ClusterID,
		"pages", len(dc.PagePoints),
	)

	// Delete old Anki cards if this cluster was previously processed
	if dc.OldState != nil && len(dc.OldState.AnkiCardIDs) > 0 {
		g.logger.Info("deleting old cards",
			"cluster_id", dc.ClusterID,
			"old_topic", dc.OldState.Topic,
			"card_count", len(dc.OldState.AnkiCardIDs),
		)
		if err := g.anki.DeleteNotes(ctx, dc.OldState.AnkiCardIDs); err != nil {
			g.logger.Warn("failed to delete old cards, continuing", "error", err)
		}
	}

	// Prepare primary pages
	primaryPages := make([]flashcardgen.PageInput, len(dc.PagePoints))
	for i, pp := range dc.PagePoints {
		primaryPages[i] = flashcardgen.PageInput{
			NotebookName:  pp.NotebookName,
			PageNumber:    int(pp.PageNumber),
			Transcription: pp.Transcription,
		}
	}

	// Fetch bridge pages (K nearest from other clusters)
	excludeIDs := make([]string, len(dc.PagePoints))
	for i, pp := range dc.PagePoints {
		excludeIDs[i] = pp.ID
	}

	centroid := CentroidVector(dc.PagePoints)
	var bridgePages []flashcardgen.PageInput
	if centroid != nil {
		neighbors, err := g.store.Search(ctx, centroid, bridgePageLimit, excludeIDs)
		if err != nil {
			g.logger.Warn("failed to fetch bridge pages", "error", err)
		} else {
			for _, n := range neighbors {
				bridgePages = append(bridgePages, flashcardgen.PageInput{
					NotebookName:  n.NotebookName,
					PageNumber:    int(n.PageNumber),
					Transcription: n.Transcription,
				})
			}
		}
	}

	// Generate topic label + flashcards via Claude
	result, err := g.generator.Generate(ctx, primaryPages, bridgePages)
	if err != nil {
		return fmt.Errorf("generate flashcards cluster %d: %w", dc.ClusterID, err)
	}

	g.logger.Info("generated flashcards",
		"cluster_id", dc.ClusterID,
		"topic", result.Topic,
		"card_count", len(result.Flashcards),
	)

	// Create Anki deck and push cards
	deckName := g.anki.DeckName(result.Topic)
	if err := g.anki.EnsureDeck(ctx, deckName); err != nil {
		return fmt.Errorf("ensure deck %q: %w", deckName, err)
	}

	notes := make([]anki.Note, len(result.Flashcards))
	for i, fc := range result.Flashcards {
		tags := append(fc.Tags, "mate", fmt.Sprintf("cluster:%s", result.Topic))
		notes[i] = anki.Note{
			DeckName:  deckName,
			ModelName: "Basic",
			Front:     fc.Front,
			Back:      fc.Back,
			Tags:      tags,
		}
	}

	cardIDs, err := g.anki.AddNotes(ctx, notes)
	if err != nil {
		return fmt.Errorf("add notes to Anki: %w", err)
	}

	// Filter out failed card IDs (0 means duplicate/failed)
	var validCardIDs []int64
	for _, id := range cardIDs {
		if id != 0 {
			validCardIDs = append(validCardIDs, id)
		}
	}

	// Save cluster state
	pageIDs := make([]string, len(dc.PagePoints))
	for i, pp := range dc.PagePoints {
		pageIDs[i] = pp.ID
	}

	if err := g.stateDB.SetCluster(dc.ClusterID, &statedb.ClusterState{
		Topic:       result.Topic,
		PageIDs:     pageIDs,
		ContentHash: dc.ContentHash,
		AnkiCardIDs: validCardIDs,
	}); err != nil {
		return fmt.Errorf("save cluster state: %w", err)
	}

	// Update page states
	for _, pp := range dc.PagePoints {
		if err := g.stateDB.SetPage(pp.ID, &statedb.PageState{
			ContentHash: pp.ContentHash,
			ClusterID:   dc.ClusterID,
		}); err != nil {
			g.logger.Warn("failed to save page state", "page_id", pp.ID, "error", err)
		}
	}

	g.logger.Info("cluster flashcards pushed to Anki",
		"cluster_id", dc.ClusterID,
		"topic", result.Topic,
		"deck", deckName,
		"cards_added", len(validCardIDs),
	)

	return nil
}

// CleanupStaleCluster removes Anki cards and state for a cluster that no longer exists.
func (g *GenerateFlashcards) CleanupStaleCluster(ctx context.Context, sc StaleCluster) error {
	g.logger.Info("cleaning up stale cluster",
		"cluster_id", sc.ClusterID,
		"topic", sc.OldState.Topic,
	)

	if len(sc.OldState.AnkiCardIDs) > 0 {
		if err := g.anki.DeleteNotes(ctx, sc.OldState.AnkiCardIDs); err != nil {
			g.logger.Warn("failed to delete stale cards", "error", err)
		}
	}

	if err := g.stateDB.DeleteCluster(sc.ClusterID); err != nil {
		return fmt.Errorf("delete stale cluster state: %w", err)
	}

	return nil
}
