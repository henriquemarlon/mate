package drive

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// File represents a file found in Google Drive.
type File struct {
	ID   string
	Name string
	// MimeType is the MIME type of the file (e.g. "application/pdf").
	MimeType string
}

// Client wraps the Google Drive API for listing and downloading files.
type Client struct {
	service  *drive.Service
	folderID string
}

// NewClient creates a new Drive client authenticated with a service account.
// credentialsJSON is the raw JSON content of the service account key file.
func NewClient(ctx context.Context, credentialsJSON string, folderID string) (*Client, error) {
	svc, err := drive.NewService(ctx, option.WithCredentialsJSON([]byte(credentialsJSON)))
	if err != nil {
		return nil, fmt.Errorf("drive: create service: %w", err)
	}
	return &Client{service: svc, folderID: folderID}, nil
}

// ListPDFs returns all PDF files in the configured folder.
func (c *Client) ListPDFs(ctx context.Context) ([]File, error) {
	query := fmt.Sprintf("'%s' in parents and mimeType='application/pdf' and trashed=false", c.folderID)

	var files []File
	var pageToken string

	for {
		call := c.service.Files.List().
			Context(ctx).
			Q(query).
			Fields("nextPageToken, files(id, name, mimeType)").
			PageSize(100)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("drive: list files: %w", err)
		}

		for _, f := range resp.Files {
			files = append(files, File{
				ID:       f.Id,
				Name:     f.Name,
				MimeType: f.MimeType,
			})
		}

		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return files, nil
}

// Download downloads a file by ID and writes its content to w.
func (c *Client) Download(ctx context.Context, fileID string, w io.Writer) error {
	resp, err := c.service.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return fmt.Errorf("drive: download %s: %w", fileID, err)
	}
	defer resp.Body.Close()

	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("drive: read body %s: %w", fileID, err)
	}
	return nil
}
