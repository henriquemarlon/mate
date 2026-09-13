package review

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/henriquemarlon/mate/configs"
	"github.com/henriquemarlon/mate/internal/domain/entity"
	"github.com/henriquemarlon/mate/internal/infra/anki"
	"github.com/henriquemarlon/mate/internal/infra/repository/sqlite"
	"github.com/henriquemarlon/mate/internal/service"
	"github.com/henriquemarlon/mate/pkg/llm"
	pkgservice "github.com/henriquemarlon/mate/pkg/service"
	"github.com/spf13/cobra"
)

var (
	cfg        *configs.MateConfig
	noteID     string
	pageNumber int
)

var Cmd = &cobra.Command{
	Use:   "review",
	Short: "Resolve pages that need human review",
	Long: `Walks the pages the transcriber could not confirm, opens the annotated
image of each one, and applies the correction you choose. Resolving a page
resumes the same material generation and Anki synchronization the unattended
run would have performed.

Every flag can also be set through its MATE_ environment variable; the
generated reference in docs/config.md lists them with their defaults.`,
	Example: reviewExamples,
	Args:    cobra.NoArgs,
	RunE:    run,
}

const reviewExamples = `# Resolve every pending page:
mate review

# Resolve one page of one notebook:
mate review --note "Distributed Systems.pdf" --page 3`

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&noteID, "note", "", "Review only this PDF path relative to the study directory")
	flags.IntVar(&pageNumber, "page", 0, "Review only this page number")

	Cmd.PreRunE = func(_ *cobra.Command, _ []string) error {
		var err error
		cfg, err = configs.LoadMateConfig()
		return err
	}
}

// run acquires the same external resources as the run command and then walks
// the pending pages, applying the correction chosen for each one, instead of
// serving on a timer.
func run(cmd *cobra.Command, _ []string) (err error) {
	ctx := cmd.Context()
	logger := pkgservice.NewLogger(service.ServiceName, cfg.LogLevel, cfg.LogColor)

	repo, err := sqlite.NewSQLiteRepository(ctx, cfg.StateDB)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, repo.Close())
	}()

	llmClient, err := llm.New(llm.Config{
		APIKey:         cfg.LLMAPIKey.Value,
		Model:          cfg.LLMModel,
		BaseURL:        cfg.LLMBaseURL,
		RequestTimeout: cfg.LLMTimeoutSeconds,
		Logger:         logger,
	})
	if err != nil {
		return fmt.Errorf("configure llm client: %w", err)
	}
	ankiClient, err := anki.New(cfg.AnkiEndpoint, cfg.AnkiDeck)
	if err != nil {
		return err
	}
	mate, err := service.Create(ctx, &service.CreateInfo{
		Config:     *cfg,
		Logger:     logger,
		Repository: repo,
		LLM:        llmClient,
		Anki:       ankiClient,
	})
	if err != nil {
		return err
	}

	output := cmd.OutOrStdout()
	items, err := mate.PendingReviews(noteID, pageNumber)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(output, "No pages need review.")
		return nil
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	items, quit, err := chooseNotebook(reader, output, items)
	if errors.Is(err, io.EOF) || quit {
		return nil
	}
	if err != nil {
		return err
	}
	for _, item := range items {
		showReviewImage(output, item)

		for item.Status == entity.PageStatusNeedsReview {
			answer, askErr := ask(reader, output, prompt(item))
			if errors.Is(askErr, io.EOF) {
				return nil
			}
			if askErr != nil {
				return askErr
			}

			// Every branch below keeps the page it is working on until the
			// service returns a new one. Assigning straight into item would
			// replace it with the zero value on failure, ending the loop as
			// if the page had been resolved.
			switch answer {
			case "q", "quit":
				return nil
			case "o", "open":
				showReviewImage(output, item)
			case "r", "retry":
				updated, retryErr := mate.RetryReview(ctx, item.NoteID, item.PageNumber)
				if retryErr != nil {
					fmt.Fprintf(output, "Retry failed: %v\n", retryErr)
					continue
				}
				item = updated
				if item.Status == entity.PageStatusNeedsReview {
					fmt.Fprintln(output, "The new transcription still needs review.")
					showReviewImage(output, item)
				}
			case "s", "skip":
				updated, skipErr := mate.SkipReview(ctx, item.NoteID, item.PageNumber)
				if skipErr != nil {
					fmt.Fprintf(output, "Skip failed: %v\n", skipErr)
					continue
				}
				item = updated
			case "k", "keep":
				updated, keepErr := mate.KeepReview(item.NoteID, item.PageNumber)
				if keepErr != nil {
					fmt.Fprintf(output, "Keep failed: %v\n", keepErr)
					continue
				}
				item = updated
			case "e", "edit":
				markdown, editErr := editTranscription(item.Transcription)
				if editErr != nil {
					fmt.Fprintf(output, "Edit failed: %v\n", editErr)
					continue
				}
				// A transcription that still carries [?] is a draft: it is
				// stored so the work is not lost, but the page stays in
				// review instead of reaching generated material.
				markdown = strings.TrimSpace(markdown)
				var updated service.ReviewItem
				if strings.Contains(markdown, "[?]") {
					updated, editErr = mate.UpdateReviewDraft(item.NoteID, item.PageNumber, markdown)
				} else {
					updated, editErr = mate.CorrectReview(ctx, item.NoteID, item.PageNumber, markdown)
				}
				if editErr != nil {
					fmt.Fprintf(output, "Correction failed: %v\n", editErr)
					continue
				}
				item = updated
			default:
				fmt.Fprintln(output, "Choose one of the options above.")
			}
		}
		fmt.Fprintf(output, "Resolved %s page %d (%s).\n", item.NoteID, item.PageNumber, item.Status)
	}
	return nil
}

// prompt lists the keys a page accepts. A page that already produced material
// can keep its previous content; a page that never did can be skipped.
func prompt(item service.ReviewItem) string {
	if item.Changed {
		return "[e]dit  [r]etry  [k]eep previous  [o]pen  [q]uit > "
	}
	return "[e]dit  [r]etry  [s]kip  [o]pen  [q]uit > "
}

// chooseNotebook narrows a session spanning several notebooks down to one, so
// their exact file names never have to be typed.
func chooseNotebook(reader *bufio.Reader, output io.Writer, items []service.ReviewItem) ([]service.ReviewItem, bool, error) {
	counts := make(map[string]int)
	order := make([]string, 0)
	for _, item := range items {
		if _, found := counts[item.NoteID]; !found {
			order = append(order, item.NoteID)
		}
		counts[item.NoteID]++
	}
	if noteID != "" || len(order) <= 1 {
		return items, false, nil
	}

	all := len(order) + 1
	for {
		fmt.Fprintln(output, "\nNotebooks needing review:")
		for index, id := range order {
			label := "pages"
			if counts[id] == 1 {
				label = "page"
			}
			fmt.Fprintf(output, "  %d) %s — %d %s\n", index+1, strings.TrimSuffix(id, filepath.Ext(id)), counts[id], label)
		}
		fmt.Fprintf(output, "  %d) Review all\n", all)

		answer, err := ask(reader, output, "Choose a notebook, or [q]uit > ")
		if err != nil {
			return nil, false, err
		}
		if answer == "q" || answer == "quit" {
			return nil, true, nil
		}
		chosen, err := strconv.Atoi(answer)
		if err != nil || chosen < 1 || chosen > all {
			fmt.Fprintf(output, "Choose a number between 1 and %d.\n", all)
			continue
		}
		if chosen == all {
			return items, false, nil
		}
		selected := make([]service.ReviewItem, 0, counts[order[chosen-1]])
		for _, item := range items {
			if item.NoteID == order[chosen-1] {
				selected = append(selected, item)
			}
		}
		return selected, false, nil
	}
}

// ask reads one answer, folded to lower case. An empty line re-prompts, so
// the same call works whether the answers are typed or piped in.
func ask(reader *bufio.Reader, output io.Writer, prompt string) (string, error) {
	for {
		fmt.Fprint(output, prompt)
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read response: %w", err)
		}
		if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "" {
			return answer, nil
		}
		if errors.Is(err, io.EOF) {
			return "", io.EOF
		}
	}
}

func showReviewImage(output io.Writer, item service.ReviewItem) {
	if err := openImage(item.ImagePath); err != nil {
		fmt.Fprintf(output, "Could not open the review image: %v\nPath: %s\n", err, item.ImagePath)
	}
}

// openImage hands the annotated page to the macOS viewer. Mate is built and
// shipped for macOS only, so there is no second viewer to reach for.
func openImage(path string) error {
	if err := exec.Command("/usr/bin/open", path).Run(); err != nil {
		return fmt.Errorf("run /usr/bin/open: %w", err)
	}
	return nil
}

// editTranscription hands the transcription to $EDITOR through a temporary
// file and returns what was saved, rejecting an emptied-out page.
func editTranscription(initial string) (string, error) {
	file, err := os.CreateTemp("", "mate-review-*.md")
	if err != nil {
		return "", fmt.Errorf("create temporary transcription: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.WriteString(initial); err != nil {
		file.Close()
		return "", fmt.Errorf("write temporary transcription: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temporary transcription: %w", err)
	}

	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "/usr/bin/vi"
	}
	parts := strings.Fields(editor)
	command := exec.Command(parts[0], append(parts[1:], path)...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("run editor: %w", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read edited transcription: %w", err)
	}
	if strings.TrimSpace(string(content)) == "" {
		return "", errors.New("edited transcription is empty")
	}
	return string(content), nil
}
