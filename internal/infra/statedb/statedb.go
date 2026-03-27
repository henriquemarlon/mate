package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	clustersBucket = []byte("clusters")
	pagesBucket    = []byte("pages")
)

// ClusterState tracks the state of a single cluster across sync runs.
type ClusterState struct {
	Topic       string   `json:"topic"`
	PageIDs     []string `json:"page_ids"`
	ContentHash string   `json:"content_hash"`
	AnkiCardIDs []int64  `json:"anki_card_ids"`
	UpdatedAt   string   `json:"updated_at"`
}

// PageState tracks which cluster a page belongs to and its content hash.
type PageState struct {
	ContentHash string `json:"content_hash"`
	ClusterID   int    `json:"cluster_id"`
	ProcessedAt string `json:"processed_at"`
}

// DB wraps BoltDB for cluster and page state tracking.
type DB struct {
	db *bolt.DB
}

// Open opens (or creates) the BoltDB state database at the given path.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("statedb: create dir: %w", err)
	}

	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("statedb: open: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(clustersBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(pagesBucket); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("statedb: init buckets: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.db.Close()
}

// GetCluster retrieves the state of a cluster by ID. Returns nil if not found.
func (d *DB) GetCluster(clusterID int) (*ClusterState, error) {
	var state *ClusterState
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(clustersBucket)
		v := b.Get([]byte(strconv.Itoa(clusterID)))
		if v == nil {
			return nil
		}
		state = &ClusterState{}
		return json.Unmarshal(v, state)
	})
	if err != nil {
		return nil, fmt.Errorf("statedb: get cluster %d: %w", clusterID, err)
	}
	return state, nil
}

// SetCluster stores the state of a cluster.
func (d *DB) SetCluster(clusterID int, state *ClusterState) error {
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("statedb: marshal cluster: %w", err)
	}
	return d.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(clustersBucket).Put([]byte(strconv.Itoa(clusterID)), data)
	})
}

// DeleteCluster removes a cluster's state.
func (d *DB) DeleteCluster(clusterID int) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(clustersBucket).Delete([]byte(strconv.Itoa(clusterID)))
	})
}

// AllClusters returns all stored cluster states.
func (d *DB) AllClusters() (map[int]*ClusterState, error) {
	clusters := make(map[int]*ClusterState)
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(clustersBucket)
		return b.ForEach(func(k, v []byte) error {
			id, err := strconv.Atoi(string(k))
			if err != nil {
				return nil // skip invalid keys
			}
			var state ClusterState
			if err := json.Unmarshal(v, &state); err != nil {
				return nil // skip corrupted entries
			}
			clusters[id] = &state
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("statedb: all clusters: %w", err)
	}
	return clusters, nil
}

// GetPage retrieves the state of a page by its ID (notebookID:pageNum).
func (d *DB) GetPage(pageID string) (*PageState, error) {
	var state *PageState
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(pagesBucket)
		v := b.Get([]byte(pageID))
		if v == nil {
			return nil
		}
		state = &PageState{}
		return json.Unmarshal(v, state)
	})
	if err != nil {
		return nil, fmt.Errorf("statedb: get page %s: %w", pageID, err)
	}
	return state, nil
}

// SetPage stores the state of a page.
func (d *DB) SetPage(pageID string, state *PageState) error {
	state.ProcessedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("statedb: marshal page: %w", err)
	}
	return d.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(pagesBucket).Put([]byte(pageID), data)
	})
}
