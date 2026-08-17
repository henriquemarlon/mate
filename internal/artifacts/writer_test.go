package artifacts

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReviewPageAnnotatesNormalizedBox(t *testing.T) {
	page := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			page.Set(x, y, color.White)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, page); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if err := WriteReviewPage(root, "Networks/note.pdf", 2, encoded.Bytes(), [][]int{{100, 100, 500, 500}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "Networks", "note", "review", "page-2.png"))
	if err != nil {
		t.Fatal(err)
	}
	annotated, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := annotated.At(10, 10).RGBA()
	if r <= g || r <= b {
		t.Fatalf("expected red border pixel, got r=%d g=%d b=%d", r, g, b)
	}
}
