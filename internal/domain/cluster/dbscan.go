package cluster

import "math"

// Point represents a data point with an identifier and embedding vector.
type Point struct {
	ID     string
	Vector []float32
}

// Result holds the output of DBSCAN clustering.
type Result struct {
	// Clusters maps cluster ID (0-based) to the list of points in that cluster.
	Clusters map[int][]Point
	// Noise contains points not assigned to any cluster.
	Noise []Point
}

// DBSCAN runs the DBSCAN clustering algorithm on the given points using cosine distance.
//
// Parameters:
//   - points: the data points to cluster
//   - epsilon: maximum cosine distance (1 - cosine_similarity) for two points to be neighbors
//   - minPoints: minimum number of points to form a dense region (cluster core)
func DBSCAN(points []Point, epsilon float64, minPoints int) Result {
	n := len(points)
	if n == 0 {
		return Result{Clusters: make(map[int][]Point)}
	}

	const (
		undefined = 0
		noise     = -1
	)

	labels := make([]int, n)    // 0 = undefined, -1 = noise, >0 = cluster ID (1-based internally)
	clusterID := 0

	for i := 0; i < n; i++ {
		if labels[i] != undefined {
			continue
		}

		neighbors := rangeQuery(points, i, epsilon)
		if len(neighbors) < minPoints {
			labels[i] = noise
			continue
		}

		clusterID++
		labels[i] = clusterID

		seed := make([]int, len(neighbors))
		copy(seed, neighbors)

		for j := 0; j < len(seed); j++ {
			q := seed[j]
			if q == i {
				continue
			}
			if labels[q] == noise {
				labels[q] = clusterID
			}
			if labels[q] != undefined {
				continue
			}
			labels[q] = clusterID

			qNeighbors := rangeQuery(points, q, epsilon)
			if len(qNeighbors) >= minPoints {
				seed = append(seed, qNeighbors...)
			}
		}
	}

	result := Result{
		Clusters: make(map[int][]Point),
	}
	for i, label := range labels {
		if label == noise {
			result.Noise = append(result.Noise, points[i])
		} else {
			cid := label - 1 // convert to 0-based
			result.Clusters[cid] = append(result.Clusters[cid], points[i])
		}
	}
	return result
}

// rangeQuery returns indices of all points within epsilon cosine distance of points[idx].
func rangeQuery(points []Point, idx int, epsilon float64) []int {
	var neighbors []int
	for i := range points {
		if cosineDistance(points[idx].Vector, points[i].Vector) <= epsilon {
			neighbors = append(neighbors, i)
		}
	}
	return neighbors
}

// cosineDistance returns 1 - cosine_similarity(a, b).
// Returns 1.0 (max distance) if either vector has zero magnitude.
func cosineDistance(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 1.0
	}

	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}

	if normA == 0 || normB == 0 {
		return 1.0
	}

	similarity := dot / (math.Sqrt(normA) * math.Sqrt(normB))
	// Clamp to [-1, 1] to handle floating point imprecision.
	if similarity > 1.0 {
		similarity = 1.0
	} else if similarity < -1.0 {
		similarity = -1.0
	}
	return 1.0 - similarity
}
