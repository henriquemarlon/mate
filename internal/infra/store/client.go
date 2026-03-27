package store

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
)

// PagePoint represents a page stored as a vector point in Qdrant.
type PagePoint struct {
	ID           string
	NotebookID   string
	NotebookName string
	PageNumber   int64
	Transcription string
	ContentHash  string
	Vector       []float32
}

// Client wraps the Qdrant gRPC client for vector storage and search.
type Client struct {
	client     *qdrant.Client
	collection string
}

// NewClient creates a new Qdrant store client.
func NewClient(address, collection string) (*Client, error) {
	host, port, err := parseAddress(address)
	if err != nil {
		return nil, fmt.Errorf("store: parse address: %w", err)
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: host,
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}

	return &Client{client: client, collection: collection}, nil
}

// EnsureCollection creates the collection if it doesn't exist.
// vectorSize is the dimensionality of the embedding vectors.
func (c *Client) EnsureCollection(ctx context.Context, vectorSize uint64) error {
	exists, err := c.client.CollectionExists(ctx, c.collection)
	if err != nil {
		return fmt.Errorf("store: check collection: %w", err)
	}
	if exists {
		return nil
	}

	err = c.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: c.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     vectorSize,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("store: create collection: %w", err)
	}
	return nil
}

// Upsert stores a page vector with metadata in Qdrant.
func (c *Client) Upsert(ctx context.Context, p PagePoint) error {
	vectors := make([]float32, len(p.Vector))
	copy(vectors, p.Vector)

	waitTrue := true
	_, err := c.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: c.collection,
		Wait:           &waitTrue,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewID(p.ID),
				Vectors: qdrant.NewVectors(vectors...),
				Payload: qdrant.NewValueMap(map[string]any{
					"notebook_id":   p.NotebookID,
					"notebook_name": p.NotebookName,
					"page_number":   p.PageNumber,
					"transcription": p.Transcription,
					"content_hash":  p.ContentHash,
				}),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("store: upsert %s: %w", p.ID, err)
	}
	return nil
}

// ScrollAll retrieves all points from the collection, paginating through all results.
func (c *Client) ScrollAll(ctx context.Context) ([]PagePoint, error) {
	var all []PagePoint
	var offset *qdrant.PointId
	batchSize := uint32(100)

	withPayload := true
	withVectors := true

	for {
		points, nextOffset, err := c.client.ScrollAndOffset(ctx, &qdrant.ScrollPoints{
			CollectionName: c.collection,
			Offset:         offset,
			Limit:          &batchSize,
			WithPayload: &qdrant.WithPayloadSelector{
				SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: withPayload},
			},
			WithVectors: &qdrant.WithVectorsSelector{
				SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: withVectors},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("store: scroll: %w", err)
		}

		for _, pt := range points {
			pp := pointToPagePoint(pt)
			all = append(all, pp)
		}

		if nextOffset == nil || len(points) == 0 {
			break
		}
		offset = nextOffset
	}

	return all, nil
}

// Search finds the K nearest points to the given vector, optionally excluding specific IDs.
func (c *Client) Search(ctx context.Context, vector []float32, limit uint64, excludeIDs []string) ([]PagePoint, error) {
	queryVec := make([]float32, len(vector))
	copy(queryVec, vector)

	withPayload := true

	req := &qdrant.QueryPoints{
		CollectionName: c.collection,
		Query:          qdrant.NewQuery(queryVec...),
		Limit:          &limit,
		WithPayload: &qdrant.WithPayloadSelector{
			SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: withPayload},
		},
	}

	if len(excludeIDs) > 0 {
		ids := make([]*qdrant.PointId, len(excludeIDs))
		for i, id := range excludeIDs {
			ids[i] = qdrant.NewID(id)
		}
		req.Filter = &qdrant.Filter{
			MustNot: []*qdrant.Condition{
				{
					ConditionOneOf: &qdrant.Condition_HasId{
						HasId: &qdrant.HasIdCondition{
							HasId: ids,
						},
					},
				},
			},
		}
	}

	scored, err := c.client.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("store: search: %w", err)
	}

	results := make([]PagePoint, 0, len(scored))
	for _, sp := range scored {
		results = append(results, scoredToPagePoint(sp))
	}
	return results, nil
}

func pointToPagePoint(pt *qdrant.RetrievedPoint) PagePoint {
	pp := PagePoint{
		ID: pointIDToString(pt.Id),
	}
	if pt.Vectors != nil {
		if v := pt.Vectors.GetVector(); v != nil {
			pp.Vector = v.Data
		}
	}
	extractPayload(pt.Payload, &pp)
	return pp
}

func scoredToPagePoint(sp *qdrant.ScoredPoint) PagePoint {
	pp := PagePoint{
		ID: pointIDToString(sp.Id),
	}
	extractPayload(sp.Payload, &pp)
	return pp
}

func extractPayload(payload map[string]*qdrant.Value, pp *PagePoint) {
	if v, ok := payload["notebook_id"]; ok {
		pp.NotebookID = v.GetStringValue()
	}
	if v, ok := payload["notebook_name"]; ok {
		pp.NotebookName = v.GetStringValue()
	}
	if v, ok := payload["page_number"]; ok {
		pp.PageNumber = v.GetIntegerValue()
	}
	if v, ok := payload["transcription"]; ok {
		pp.Transcription = v.GetStringValue()
	}
	if v, ok := payload["content_hash"]; ok {
		pp.ContentHash = v.GetStringValue()
	}
}

func pointIDToString(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	if uuid := id.GetUuid(); uuid != "" {
		return uuid
	}
	return fmt.Sprintf("%d", id.GetNum())
}

func parseAddress(addr string) (string, int, error) {
	// Simple host:port parser
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			host := addr[:i]
			portStr := addr[i+1:]
			port := 0
			for _, c := range portStr {
				if c < '0' || c > '9' {
					return "", 0, fmt.Errorf("invalid port in address %q", addr)
				}
				port = port*10 + int(c-'0')
			}
			if host == "" {
				host = "localhost"
			}
			return host, port, nil
		}
	}
	return addr, 6334, nil
}
