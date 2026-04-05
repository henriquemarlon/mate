package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strconv"

	"github.com/henriquemarlon/mate/pkg/dbscan"
	"github.com/henriquemarlon/mate/internal/infra/statedb"
	"github.com/henriquemarlon/mate/internal/infra/store"
)

type DirtyCluster struct {
	ClusterID   int
	PagePoints  []store.PagePoint
	ContentHash string
	OldState    *statedb.ClusterState
}

type StaleCluster struct {
	ClusterID int
	OldState  *statedb.ClusterState
}

type ClusterNotes struct {
	store   *store.Client
	stateDB *statedb.DB
	epsilon float64
	minPts  int
	logger  *slog.Logger
}

func NewClusterNotes(
	storeClient *store.Client,
	stateDB *statedb.DB,
	epsilon float64,
	minPts int,
	logger *slog.Logger,
) *ClusterNotes {
	return &ClusterNotes{
		store:   storeClient,
		stateDB: stateDB,
		epsilon: epsilon,
		minPts:  minPts,
		logger:  logger,
	}
}

func (c *ClusterNotes) Execute(ctx context.Context) ([]DirtyCluster, []StaleCluster, error) {
	allPoints, err := c.store.ScrollAll(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster: scroll all: %w", err)
	}
	c.logger.Info("loaded all points from Qdrant", "count", len(allPoints))

	if len(allPoints) == 0 {
		return nil, nil, nil
	}

	dbscanPoints := make([]dbscan.Point, len(allPoints))
	pointMap := make(map[string]store.PagePoint, len(allPoints))
	for i, pp := range allPoints {
		dbscanPoints[i] = dbscan.Point{
			ID:     pp.ID,
			Vector: pp.Vector,
		}
		pointMap[pp.ID] = pp
	}

	result := dbscan.DBSCAN(dbscanPoints, c.epsilon, c.minPts)
	c.logger.Info("DBSCAN complete",
		"clusters", len(result.Clusters),
		"noise_points", len(result.Noise),
	)

	oldClusters, err := c.stateDB.AllClusters()
	if err != nil {
		return nil, nil, fmt.Errorf("cluster: load old state: %w", err)
	}

	var dirty []DirtyCluster
	seenClusterIDs := make(map[int]bool)

	for clusterID, points := range result.Clusters {
		seenClusterIDs[clusterID] = true

		pagePoints := make([]store.PagePoint, 0, len(points))
		for _, pt := range points {
			if pp, ok := pointMap[pt.ID]; ok {
				pagePoints = append(pagePoints, pp)
			}
		}

		contentHash := computeClusterHash(pagePoints)

		oldState := oldClusters[clusterID]
		isDirty := false

		if oldState == nil {
			isDirty = true
			c.logger.Info("new cluster detected", "cluster_id", clusterID, "pages", len(pagePoints))
		} else if oldState.ContentHash != contentHash {
			isDirty = true
			c.logger.Info("cluster content changed", "cluster_id", clusterID, "pages", len(pagePoints))
		}

		if isDirty {
			dirty = append(dirty, DirtyCluster{
				ClusterID:   clusterID,
				PagePoints:  pagePoints,
				ContentHash: contentHash,
				OldState:    oldState,
			})
		}
	}

	var stale []StaleCluster
	for oldID, oldState := range oldClusters {
		if !seenClusterIDs[oldID] {
			c.logger.Info("stale cluster detected", "cluster_id", oldID, "topic", oldState.Topic)
			stale = append(stale, StaleCluster{
				ClusterID: oldID,
				OldState:  oldState,
			})
		}
	}

	return dirty, stale, nil
}

func CentroidVector(points []store.PagePoint) []float32 {
	if len(points) == 0 {
		return nil
	}

	dim := len(points[0].Vector)
	centroid := make([]float64, dim)
	for _, pp := range points {
		for i, v := range pp.Vector {
			centroid[i] += float64(v)
		}
	}

	n := float64(len(points))
	result := make([]float32, dim)
	for i := range centroid {
		result[i] = float32(centroid[i] / n)
	}
	return result
}

func computeClusterHash(points []store.PagePoint) string {
	hashes := make([]string, len(points))
	for i, pp := range points {
		hashes[i] = pp.ContentHash
	}
	sort.Strings(hashes)

	h := sha256.New()
	for _, hash := range hashes {
		h.Write([]byte(hash))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func ParseDBSCANParams(epsilonStr, minPointsStr string) (float64, int, error) {
	epsilon, err := strconv.ParseFloat(epsilonStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse epsilon %q: %w", epsilonStr, err)
	}
	minPts, err := strconv.Atoi(minPointsStr)
	if err != nil {
		return 0, 0, fmt.Errorf("parse minPoints %q: %w", minPointsStr, err)
	}
	return epsilon, minPts, nil
}
