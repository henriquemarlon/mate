package dbscan

import (
	"testing"
)

func TestDBSCAN_TwoClusters(t *testing.T) {
	points := []Point{
		{ID: "a1", Vector: []float32{1, 0, 0}},
		{ID: "a2", Vector: []float32{0.95, 0.05, 0}},
		{ID: "a3", Vector: []float32{0.9, 0.1, 0}},
		{ID: "b1", Vector: []float32{0, 0, 1}},
		{ID: "b2", Vector: []float32{0, 0.05, 0.95}},
		{ID: "b3", Vector: []float32{0, 0.1, 0.9}},
	}

	result := DBSCAN(points, 0.15, 2)

	if len(result.Clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(result.Clusters))
	}
	if len(result.Noise) != 0 {
		t.Fatalf("expected 0 noise points, got %d", len(result.Noise))
	}
}

func TestDBSCAN_WithNoise(t *testing.T) {
	points := []Point{
		{ID: "a1", Vector: []float32{1, 0, 0}},
		{ID: "a2", Vector: []float32{0.95, 0.05, 0}},
		{ID: "outlier", Vector: []float32{0.5, 0.5, 0.5}},
	}

	result := DBSCAN(points, 0.1, 2)

	if len(result.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(result.Clusters))
	}
	if len(result.Noise) != 1 {
		t.Fatalf("expected 1 noise point, got %d", len(result.Noise))
	}
	if result.Noise[0].ID != "outlier" {
		t.Fatalf("expected noise point 'outlier', got '%s'", result.Noise[0].ID)
	}
}

func TestDBSCAN_Empty(t *testing.T) {
	result := DBSCAN(nil, 0.3, 2)
	if len(result.Clusters) != 0 {
		t.Fatalf("expected 0 clusters for empty input, got %d", len(result.Clusters))
	}
}

func TestDBSCAN_AllNoise(t *testing.T) {
	points := []Point{
		{ID: "a", Vector: []float32{1, 0, 0}},
		{ID: "b", Vector: []float32{0, 1, 0}},
		{ID: "c", Vector: []float32{0, 0, 1}},
	}

	result := DBSCAN(points, 0.01, 2)

	if len(result.Clusters) != 0 {
		t.Fatalf("expected 0 clusters, got %d", len(result.Clusters))
	}
	if len(result.Noise) != 3 {
		t.Fatalf("expected 3 noise points, got %d", len(result.Noise))
	}
}

func TestCosineDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float64
		tol  float64
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 0.0, 1e-9},
		{"orthogonal", []float32{1, 0, 0}, []float32{0, 1, 0}, 1.0, 1e-9},
		{"opposite", []float32{1, 0, 0}, []float32{-1, 0, 0}, 2.0, 1e-9},
		{"zero_vector", []float32{0, 0, 0}, []float32{1, 0, 0}, 1.0, 1e-9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineDistance(tt.a, tt.b)
			if diff := got - tt.want; diff < -tt.tol || diff > tt.tol {
				t.Errorf("CosineDistance = %f, want %f", got, tt.want)
			}
		})
	}
}
