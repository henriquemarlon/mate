package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
)

var ErrInvalidFeynmanPrompt = errors.New("invalid feynman prompt")

// legacyFeynmanTitle names the single script kept by material stored before
// prompts became a per-topic list.
const legacyFeynmanTitle = "Revisão geral"

// slugMaxLength keeps a derived file name short enough to stay portable
// across filesystems even when the title is a full sentence.
const slugMaxLength = 60

// slugFold maps the accented Latin letters that appear in note titles to
// their ASCII base. Folding here avoids a dependency for the one script the
// notes actually use.
var slugFold = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n', 'ý': 'y', 'ÿ': 'y',
}

// FeynmanPrompt is one self-contained tutoring session covering a single
// subject. The model supplies only Title, Pages, and Content; Mate derives ID
// and SourceHash so a session is identified by what it actually covers rather
// than by anything the model is free to invent.
type FeynmanPrompt struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Pages      []int  `json:"pages"`
	SourceHash string `json:"source_hash"`
	Content    string `json:"content"`
}

// Identify validates a session against the exact batch it was generated from
// and derives the fields Mate owns. source maps every page number sent to the
// model to its transcription, so a session can neither cite a page outside
// that batch nor keep a hash that survives an edit to the pages it covers.
func (f *FeynmanPrompt) Identify(source map[int]string) error {
	f.Title = strings.TrimSpace(f.Title)
	f.Content = strings.TrimSpace(f.Content)
	if f.Title == "" {
		return fmt.Errorf("%w: title cannot be empty", ErrInvalidFeynmanPrompt)
	}
	if f.Content == "" {
		return fmt.Errorf("%w: %q has no content", ErrInvalidFeynmanPrompt, f.Title)
	}
	if len(f.Pages) == 0 {
		return fmt.Errorf("%w: %q cites no pages", ErrInvalidFeynmanPrompt, f.Title)
	}

	pages := slices.Clone(f.Pages)
	slices.Sort(pages)
	pages = slices.Compact(pages)
	digest := sha256.New()
	for _, page := range pages {
		markdown, found := source[page]
		if !found {
			return fmt.Errorf("%w: %q cites page %d, which is not in this batch", ErrInvalidFeynmanPrompt, f.Title, page)
		}
		fmt.Fprintf(digest, "%d\x00%s\x00", page, markdown)
	}

	f.Pages = pages
	f.SourceHash = hex.EncodeToString(digest.Sum(nil))
	f.identify()
	return nil
}

// NewLegacyFeynmanPrompt wraps a pre-topic Markdown script so material stored
// before this change keeps working without spending another model turn. It
// covers no known pages, so it carries no source hash.
func NewLegacyFeynmanPrompt(content string) (FeynmanPrompt, error) {
	prompt := FeynmanPrompt{Title: legacyFeynmanTitle, Content: strings.TrimSpace(content)}
	if prompt.Content == "" {
		return FeynmanPrompt{}, fmt.Errorf("%w: %q has no content", ErrInvalidFeynmanPrompt, legacyFeynmanTitle)
	}
	prompt.identify()
	return prompt, nil
}

func (f FeynmanPrompt) Validate() error {
	if strings.TrimSpace(f.ID) == "" {
		return fmt.Errorf("%w: ID cannot be empty", ErrInvalidFeynmanPrompt)
	}
	if strings.TrimSpace(f.Title) == "" {
		return fmt.Errorf("%w: title cannot be empty", ErrInvalidFeynmanPrompt)
	}
	if strings.TrimSpace(f.Content) == "" {
		return fmt.Errorf("%w: %q has no content", ErrInvalidFeynmanPrompt, f.Title)
	}
	return nil
}

// identify derives the stable identity of a session. The ID doubles as the
// artifact base name so the file on disk and the record in storage can never
// drift apart, and sorts by page because that is the reading order.
func (f *FeynmanPrompt) identify() {
	if len(f.Pages) == 0 {
		f.ID = slug(f.Title)
		return
	}
	f.ID = fmt.Sprintf("%03d-%03d-%s", f.Pages[0], f.Pages[len(f.Pages)-1], slug(f.Title))
}

func slug(title string) string {
	var builder strings.Builder
	separated := true
	for _, symbol := range strings.ToLower(title) {
		if folded, found := slugFold[symbol]; found {
			symbol = folded
		}
		switch {
		case symbol >= 'a' && symbol <= 'z', symbol >= '0' && symbol <= '9':
			builder.WriteRune(symbol)
			separated = false
		case !separated:
			builder.WriteByte('-')
			separated = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if len(result) > slugMaxLength {
		result = strings.Trim(result[:slugMaxLength], "-")
	}
	if result == "" {
		return "session"
	}
	return result
}
