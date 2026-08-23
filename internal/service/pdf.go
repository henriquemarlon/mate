package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// renderedPage references one rendered page on disk. PNG bytes are read on
// demand so a large notebook never has to fit in memory at once.
type renderedPage struct {
	Number int
	Path   string
	Hash   string
}

// renderPDF rasterizes every page of the PDF into dir and returns the pages
// sorted by number. The caller owns dir and must remove it when done.
func renderPDF(ctx context.Context, pdfPath string, dpi int) (dir string, pages []renderedPage, err error) {
	dir, err = os.MkdirTemp("", "mate-render-")
	if err != nil {
		return "", nil, fmt.Errorf("workflow: create render directory: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()

	prefix := filepath.Join(dir, "page")
	output, err := exec.CommandContext(ctx, "pdftoppm", "-png", "-r", strconv.Itoa(dpi), pdfPath, prefix).CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("workflow: render PDF: %w: %s", err, strings.TrimSpace(string(output)))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, fmt.Errorf("workflow: list rendered pages: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "page-") || !strings.HasSuffix(name, ".png") {
			continue
		}
		number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "page-"), ".png"))
		if err != nil {
			continue
		}
		path := filepath.Join(dir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			return "", nil, fmt.Errorf("workflow: hash rendered page %d: %w", number, err)
		}
		digest := sha256.Sum256(content)
		pages = append(pages, renderedPage{Number: number, Path: path, Hash: hex.EncodeToString(digest[:])})
	}
	if len(pages) == 0 {
		return "", nil, fmt.Errorf("workflow: no pages rendered from %s", pdfPath)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Number < pages[j].Number })
	return dir, pages, nil
}
