package service

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
	"strconv"
	"strings"

	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/llm/paradigm"
)

func writeTranscript(root, noteID string, pages []entity.Page) error {
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

func writeMaterial(root, noteID string, material paradigm.GenerateOutputDTO) error {
	dir, err := noteDirectory(root, noteID)
	if err != nil {
		return err
	}
	if err := writeFeynmanPrompts(dir, noteID, material.Feynman); err != nil {
		return err
	}
	cards, err := json.MarshalIndent(material.Cards, "", "  ")
	if err != nil {
		return fmt.Errorf("artifacts: encode cards: %w", err)
	}
	return writeAtomic(filepath.Join(dir, "cards.json"), append(cards, '\n'))
}

// writeFeynmanPrompts gives every session its own file so one can be opened
// and pasted whole into a voice assistant, and rebuilds the index from the
// current sessions so it can never reference a file that is not there.
func writeFeynmanPrompts(dir, noteID string, prompts []entity.FeynmanPrompt) error {
	if len(prompts) == 0 {
		return nil
	}
	promptDir := filepath.Join(dir, "feynman")
	var index strings.Builder
	fmt.Fprintf(&index, "# %s\n\n", strings.TrimSuffix(filepath.Base(noteID), filepath.Ext(noteID)))
	for _, prompt := range prompts {
		if err := prompt.Validate(); err != nil {
			return fmt.Errorf("artifacts: %w", err)
		}
		name := prompt.ID + ".md"
		content := fmt.Sprintf("# %s\n\n%s\n", prompt.Title, prompt.Content)
		if err := writeAtomic(filepath.Join(promptDir, name), []byte(content)); err != nil {
			return err
		}
		fmt.Fprintf(&index, "- [%s](%s)", prompt.Title, name)
		if pages := formatPages(prompt.Pages); pages != "" {
			fmt.Fprintf(&index, " — %s", pages)
		}
		index.WriteByte('\n')
	}
	if err := writeAtomic(filepath.Join(promptDir, "index.md"), []byte(index.String())); err != nil {
		return err
	}
	// Only once every session is on disk can the pre-topic script go, so an
	// interrupted write never leaves the note without any Feynman material.
	if err := os.Remove(filepath.Join(dir, "feynman.md")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("artifacts: remove legacy feynman script: %w", err)
	}
	return nil
}

// formatPages collapses consecutive pages into ranges, so a session covering
// 3, 4 and 7 reads as "páginas 3–4, 7" instead of claiming everything between.
func formatPages(pages []int) string {
	if len(pages) == 0 {
		return ""
	}
	var ranges []string
	start := 0
	for index := 1; index <= len(pages); index++ {
		if index < len(pages) && pages[index] == pages[index-1]+1 {
			continue
		}
		if start == index-1 {
			ranges = append(ranges, strconv.Itoa(pages[start]))
		} else {
			ranges = append(ranges, fmt.Sprintf("%d–%d", pages[start], pages[index-1]))
		}
		start = index
	}
	label := "páginas"
	if len(pages) == 1 {
		label = "página"
	}
	return label + " " + strings.Join(ranges, ", ")
}

func writeReviewPage(root, noteID string, pageNumber int, pagePNG []byte, boxes [][]int) error {
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
