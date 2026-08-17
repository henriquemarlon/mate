package artifacts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/codex/paradigm"
)

func WriteTranscript(root, noteID string, pages []entity.Page) error {
	dir, err := noteDirectory(root, noteID)
	if err != nil {
		return err
	}
	var content strings.Builder
	fmt.Fprintf(&content, "# %s\n\n", strings.TrimSuffix(filepath.Base(noteID), filepath.Ext(noteID)))
	for _, page := range pages {
		if strings.TrimSpace(page.Transcription) == "" {
			continue
		}
		fmt.Fprintf(&content, "## Page %d\n\n%s\n\n", page.PageNumber, page.Transcription)
	}
	return writeAtomic(filepath.Join(dir, "transcript.md"), []byte(content.String()))
}

func WriteMaterial(root, noteID string, material paradigm.Material) error {
	dir, err := noteDirectory(root, noteID)
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dir, "feynman.md"), []byte(material.Feynman+"\n")); err != nil {
		return err
	}
	cards, err := json.MarshalIndent(material.Cards, "", "  ")
	if err != nil {
		return fmt.Errorf("artifacts: encode cards: %w", err)
	}
	return writeAtomic(filepath.Join(dir, "cards.json"), append(cards, '\n'))
}

func WriteReviewPage(root, noteID string, pageNumber int, pagePNG []byte, boxes [][]int) error {
	dir, err := noteDirectory(root, noteID)
	if err != nil {
		return err
	}
	annotated, err := annotate(pagePNG, boxes)
	if err != nil {
		return fmt.Errorf("artifacts: annotate review page: %w", err)
	}
	return writeAtomic(filepath.Join(dir, "review", fmt.Sprintf("page-%d.png", pageNumber)), annotated)
}

func noteDirectory(root, noteID string) (string, error) {
	relative := strings.TrimSuffix(filepath.FromSlash(noteID), filepath.Ext(noteID))
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("artifacts: invalid note path %q", noteID)
	}
	dir := filepath.Join(root, clean)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("artifacts: create note directory: %w", err)
	}
	return dir, nil
}

func writeAtomic(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("artifacts: create directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".mate-*")
	if err != nil {
		return fmt.Errorf("artifacts: create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("artifacts: write temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("artifacts: close temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("artifacts: publish %s: %w", path, err)
	}
	return nil
}

func annotate(pagePNG []byte, boxes [][]int) ([]byte, error) {
	if len(boxes) == 0 {
		return pagePNG, nil
	}
	decoded, err := png.Decode(bytes.NewReader(pagePNG))
	if err != nil {
		return nil, err
	}
	bounds := decoded.Bounds()
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, decoded, bounds.Min, draw.Src)
	red := color.RGBA{R: 220, G: 30, B: 30, A: 255}
	thickness := max(3, bounds.Dx()/500)
	for _, box := range boxes {
		if len(box) != 4 {
			continue
		}
		x1 := bounds.Min.X + box[0]*bounds.Dx()/1000
		y1 := bounds.Min.Y + box[1]*bounds.Dy()/1000
		x2 := bounds.Min.X + box[2]*bounds.Dx()/1000
		y2 := bounds.Min.Y + box[3]*bounds.Dy()/1000
		x1, x2 = clamp(x1, bounds.Min.X, bounds.Max.X-1), clamp(x2, bounds.Min.X, bounds.Max.X-1)
		y1, y2 = clamp(y1, bounds.Min.Y, bounds.Max.Y-1), clamp(y2, bounds.Min.Y, bounds.Max.Y-1)
		if x2 <= x1 || y2 <= y1 {
			continue
		}
		for offset := 0; offset < thickness; offset++ {
			for x := x1; x <= x2; x++ {
				canvas.Set(x, clamp(y1+offset, bounds.Min.Y, bounds.Max.Y-1), red)
				canvas.Set(x, clamp(y2-offset, bounds.Min.Y, bounds.Max.Y-1), red)
			}
			for y := y1; y <= y2; y++ {
				canvas.Set(clamp(x1+offset, bounds.Min.X, bounds.Max.X-1), y, red)
				canvas.Set(clamp(x2-offset, bounds.Min.X, bounds.Max.X-1), y, red)
			}
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
