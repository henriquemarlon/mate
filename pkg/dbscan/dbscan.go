package dbscan

import "math"

type Point struct {
	ID     string
	Vector []float32
}

type Result struct {
	Clusters map[int][]Point
	Noise    []Point
}

func DBSCAN(points []Point, epsilon float64, minPoints int) Result {
	n := len(points)
	if n == 0 {
		return Result{Clusters: make(map[int][]Point)}
	}

	const (
		undefined = 0
		noise     = -1
	)

	labels := make([]int, n)
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
			cid := label - 1
			result.Clusters[cid] = append(result.Clusters[cid], points[i])
		}
	}
	return result
}

func rangeQuery(points []Point, idx int, epsilon float64) []int {
	var neighbors []int
	for i := range points {
		if CosineDistance(points[idx].Vector, points[i].Vector) <= epsilon {
			neighbors = append(neighbors, i)
		}
	}
	return neighbors
}

func CosineDistance(a, b []float32) float64 {
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
	if similarity > 1.0 {
		similarity = 1.0
	} else if similarity < -1.0 {
		similarity = -1.0
	}
	return 1.0 - similarity
}
