package review

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	"golang.org/x/term"
)

type menuChoice string

const (
	choiceCorrect   menuChoice = "correct"
	choiceRetry     menuChoice = "retry"
	choiceEdit      menuChoice = "edit"
	choiceSkip      menuChoice = "skip"
	choiceKeep      menuChoice = "keep"
	choiceOpen      menuChoice = "open"
	choiceQuit      menuChoice = "quit"
	choiceReviewAll menuChoice = "review-all"
)

type menuOption struct {
	Label string
	Value menuChoice
}

var (
	cfg        *configs.MateConfig
	noteID     string
	pageNumber int
	noOpen     bool
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

# Print image paths instead of opening them:
mate review --no-open`

func init() {
	flags := Cmd.Flags()
	flags.StringVar(&noteID, "note", "", "Review only this PDF path relative to the study directory")
	flags.IntVar(&pageNumber, "page", 0, "Review only this page number")
	flags.BoolVar(&noOpen, "no-open", false, "Print image paths without opening them")

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
	input := cmd.InOrStdin()
	reader := bufio.NewReader(input)
	terminalFD, interactive := interactiveTerminal(input, output)
	items, quit, err := chooseNotebook(reader, output, terminalFD, interactive, items)
	if err != nil {
		return err
	}
	if quit {
		return nil
	}
	for _, item := range items {
		fmt.Fprintf(output, "\n%s — page %d\n", item.NoteID, item.PageNumber)
		if item.Changed {
			fmt.Fprintln(output, "Reason: a previously processed page changed.")
		} else {
			fmt.Fprintln(output, "Reason: the transcription needs human confirmation.")
		}
		showReviewImage(output, item)

		for item.Status == entity.PageStatusNeedsReview {
			if excerpt := uncertaintyExcerpt(item.Transcription); excerpt != "" {
				fmt.Fprintf(output, "\nUncertain excerpt:\n%s\n", excerpt)
			}
			choice, chooseErr := chooseMenu(reader, output, terminalFD, interactive, "What would you like to do?", reviewOptions(item))
			if errors.Is(chooseErr, io.EOF) {
				return nil
			}
			if chooseErr != nil {
				return chooseErr
			}

			switch choice {
			case choiceQuit:
				return nil
			case choiceOpen:
				showReviewImage(output, item)
				continue
			case choiceRetry:
				item, err = mate.RetryReview(ctx, item.NoteID, item.PageNumber)
				if err != nil {
					fmt.Fprintf(output, "Retry failed: %v\n", err)
					continue
				}
				if item.Status == entity.PageStatusNeedsReview {
					fmt.Fprintln(output, "The new transcription still needs review.")
					showReviewImage(output, item)
				}
			case choiceSkip:
				item, err = mate.SkipReview(ctx, item.NoteID, item.PageNumber)
				if err != nil {
					fmt.Fprintf(output, "Skip failed: %v\n", err)
					continue
				}
			case choiceKeep:
				item, err = mate.KeepReview(item.NoteID, item.PageNumber)
				if err != nil {
					fmt.Fprintf(output, "Keep failed: %v\n", err)
					continue
				}
			case choiceEdit:
				markdown, editErr := editTranscription(item.Transcription)
				if editErr != nil {
					fmt.Fprintf(output, "Edit failed: %v\n", editErr)
					continue
				}
				item, err = saveOrComplete(ctx, mate, item, markdown)
				if err != nil {
					fmt.Fprintf(output, "Correction failed: %v\n", err)
					continue
				}
			case choiceCorrect:
				if !strings.Contains(item.Transcription, "[?]") {
					fmt.Fprintln(output, "There is no [?] marker to replace; edit the full transcription instead.")
					continue
				}
				answer, readErr := readRequiredLine(reader, output, "Replacement for [?]: ")
				if errors.Is(readErr, io.EOF) {
					return nil
				}
				if readErr != nil {
					return readErr
				}
				markdown := strings.Replace(item.Transcription, "[?]", answer, 1)
				item, err = saveOrComplete(ctx, mate, item, markdown)
				if err != nil {
					fmt.Fprintf(output, "Correction failed: %v\n", err)
					continue
				}
			}
		}
		fmt.Fprintf(output, "Resolved %s page %d (%s).\n", item.NoteID, item.PageNumber, item.Status)
	}
	return nil
}

func chooseNotebook(reader *bufio.Reader, output io.Writer, terminalFD int, interactive bool, items []service.ReviewItem) ([]service.ReviewItem, bool, error) {
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

	options := make([]menuOption, 0, len(order)+2)
	for _, id := range order {
		name := strings.TrimSuffix(id, filepath.Ext(id))
		label := fmt.Sprintf("%s — %d %s", name, counts[id], plural(counts[id], "page", "pages"))
		options = append(options, menuOption{Label: label, Value: menuChoice(id)})
	}
	options = append(options,
		menuOption{Label: "Review all", Value: choiceReviewAll},
		menuOption{Label: "Quit", Value: choiceQuit},
	)
	choice, err := chooseMenu(reader, output, terminalFD, interactive, "Choose a notebook:", options)
	if err != nil {
		return nil, false, err
	}
	if choice == choiceQuit {
		return nil, true, nil
	}
	if choice == choiceReviewAll {
		return items, false, nil
	}
	selected := make([]service.ReviewItem, 0, counts[string(choice)])
	for _, item := range items {
		if item.NoteID == string(choice) {
			selected = append(selected, item)
		}
	}
	return selected, false, nil
}

func reviewOptions(item service.ReviewItem) []menuOption {
	options := make([]menuOption, 0, 6)
	if strings.Contains(item.Transcription, "[?]") {
		options = append(options, menuOption{Label: "Correct uncertain text", Value: choiceCorrect})
	}
	options = append(options,
		menuOption{Label: "Retry transcription", Value: choiceRetry},
		menuOption{Label: "Edit full transcription", Value: choiceEdit},
	)
	if item.Changed {
		options = append(options, menuOption{Label: "Keep previous content", Value: choiceKeep})
	} else {
		options = append(options, menuOption{Label: "Skip page", Value: choiceSkip})
	}
	return append(options,
		menuOption{Label: "Open image again", Value: choiceOpen},
		menuOption{Label: "Quit", Value: choiceQuit},
	)
}

func chooseMenu(reader *bufio.Reader, output io.Writer, terminalFD int, interactive bool, title string, options []menuOption) (menuChoice, error) {
	if len(options) == 0 {
		return "", errors.New("menu has no options")
	}
	if !interactive {
		return chooseNumbered(reader, output, title, options)
	}
	return chooseTerminal(reader, output, terminalFD, title, options)
}

func chooseTerminal(reader *bufio.Reader, output io.Writer, terminalFD int, title string, options []menuOption) (menuChoice, error) {
	state, err := term.MakeRaw(terminalFD)
	if err != nil {
		return "", fmt.Errorf("enable interactive menu: %w", err)
	}
	defer term.Restore(terminalFD, state)

	selected := 0
	fmt.Fprintf(output, "\n%s\r\n\r\n", title)
	fmt.Fprint(output, "\x1b[?25l")
	defer fmt.Fprint(output, "\x1b[?25h")
	renderMenu(output, options, selected, false)
	for {
		key, readErr := reader.ReadByte()
		if readErr != nil {
			return "", readErr
		}
		switch key {
		case 3, 4:
			return "", io.EOF
		case '\r', '\n':
			fmt.Fprint(output, "\r\n")
			return options[selected].Value, nil
		case 'k':
			selected = (selected - 1 + len(options)) % len(options)
			renderMenu(output, options, selected, true)
		case 'j':
			selected = (selected + 1) % len(options)
			renderMenu(output, options, selected, true)
		case 27:
			first, err := reader.ReadByte()
			if err != nil {
				return "", err
			}
			second, err := reader.ReadByte()
			if err != nil {
				return "", err
			}
			if first != '[' {
				continue
			}
			switch second {
			case 'A':
				selected = (selected - 1 + len(options)) % len(options)
				renderMenu(output, options, selected, true)
			case 'B':
				selected = (selected + 1) % len(options)
				renderMenu(output, options, selected, true)
			}
		}
	}
}

func renderMenu(output io.Writer, options []menuOption, selected int, redraw bool) {
	if redraw {
		fmt.Fprintf(output, "\x1b[%dA", len(options))
	}
	for index, option := range options {
		prefix := "  "
		if index == selected {
			prefix = "❯ "
		}
		fmt.Fprintf(output, "\r\x1b[2K%s%s\r\n", prefix, option.Label)
	}
}

func chooseNumbered(reader *bufio.Reader, output io.Writer, title string, options []menuOption) (menuChoice, error) {
	for {
		fmt.Fprintf(output, "\n%s\n", title)
		for index, option := range options {
			fmt.Fprintf(output, "%d) %s\n", index+1, option.Label)
		}
		answer, err := readRequiredLine(reader, output, "> ")
		if err != nil {
			return "", err
		}
		selected, err := strconv.Atoi(answer)
		if err == nil && selected >= 1 && selected <= len(options) {
			return options[selected-1].Value, nil
		}
		fmt.Fprintf(output, "Choose a number between 1 and %d.\n", len(options))
	}
}

func readRequiredLine(reader *bufio.Reader, output io.Writer, prompt string) (string, error) {
	for {
		fmt.Fprint(output, prompt)
		answer, err := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read response: %w", err)
		}
		if answer != "" {
			return answer, nil
		}
		if errors.Is(err, io.EOF) {
			return "", io.EOF
		}
		fmt.Fprintln(output, "Response cannot be empty.")
	}
}

func interactiveTerminal(input io.Reader, output io.Writer) (int, bool) {
	in, inputOK := input.(*os.File)
	out, outputOK := output.(*os.File)
	if !inputOK || !outputOK || !term.IsTerminal(int(in.Fd())) || !term.IsTerminal(int(out.Fd())) {
		return -1, false
	}
	return int(in.Fd()), true
}

func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func saveOrComplete(ctx context.Context, mate *service.Service, item service.ReviewItem, markdown string) (service.ReviewItem, error) {
	markdown = strings.TrimSpace(markdown)
	if strings.Contains(markdown, "[?]") {
		return mate.UpdateReviewDraft(item.NoteID, item.PageNumber, markdown)
	}
	return mate.CorrectReview(ctx, item.NoteID, item.PageNumber, markdown)
}

func showReviewImage(output io.Writer, item service.ReviewItem) {
	if noOpen {
		fmt.Fprintf(output, "Review image: %s\n", item.ImagePath)
		return
	}
	if err := openImage(item.ImagePath); err != nil {
		fmt.Fprintf(output, "Could not open the review image: %v\nPath: %s\n", err, item.ImagePath)
	}
}

func openImage(path string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "/usr/bin/open", []string{path}
	case "windows":
		command, args = "cmd", []string{"/c", "start", "", path}
	default:
		command, args = "xdg-open", []string{path}
	}
	if err := exec.Command(command, args...).Run(); err != nil {
		return fmt.Errorf("run %s: %w", command, err)
	}
	return nil
}

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

func uncertaintyExcerpt(markdown string) string {
	index := strings.Index(markdown, "[?]")
	if index < 0 {
		return ""
	}
	before := []rune(markdown[:index])
	after := []rune(markdown[index+len("[?]"):])
	prefix := ""
	suffix := ""
	if len(before) > 120 {
		before = before[len(before)-120:]
		prefix = "…"
	}
	if len(after) > 120 {
		after = after[:120]
		suffix = "…"
	}
	return prefix + string(before) + "[?]" + string(after) + suffix
}
