package progressbar

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	greenColor      = "\033[32m"
	resetColor      = "\033[0m"
	defaultBarWidth = 25
	punMarkerSet    = "#abcdefjklmnopqrxyz"
)

type ProgressBar interface {
	Add(name string)
	Run(name string, progress <-chan int) error
	Update(name string, percent int) error
	Finish(name string) error
}

type RenderError struct {
	Op   string
	Task string
	Err  error
}

func (e *RenderError) Error() string {
	if e.Task == "" {
		return fmt.Sprintf("progressbar render failed (%s): %v", e.Op, e.Err)
	}

	return fmt.Sprintf("progressbar render failed (%s, task=%q): %v", e.Op, e.Task, e.Err)
}

func (e *RenderError) Unwrap() error {
	return e.Err
}

type ConsoleProgressBar struct {
	width    int
	out      io.Writer
	ansi     bool
	markers  []rune
	random   *rand.Rand
	mu       sync.Mutex
	order    []string
	percent  map[string]int
	finished map[string]bool
	rendered bool
}

func NewConsoleProgressBar(width int) *ConsoleProgressBar {
	return NewConsoleProgressBarWithWriter(width, os.Stdout)
}

func NewConsoleProgressBarWithWriter(width int, out io.Writer) *ConsoleProgressBar {
	if width <= 0 {
		width = defaultBarWidth
	}
	if out == nil {
		out = os.Stdout
	}

	return &ConsoleProgressBar{
		width:    width,
		out:      out,
		ansi:     supportsANSI(out),
		markers:  []rune(punMarkerSet),
		random:   rand.New(rand.NewSource(time.Now().UnixNano())),
		percent:  make(map[string]int),
		finished: make(map[string]bool),
	}
}

func (b *ConsoleProgressBar) Add(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureTaskLocked(name)
}

func (b *ConsoleProgressBar) Run(name string, progress <-chan int) error {
	b.Add(name)

	for percent := range progress {
		if err := b.Update(name, percent); err != nil {
			return fmt.Errorf("run progress for task %q: %w", name, err)
		}
	}

	if err := b.Finish(name); err != nil {
		return fmt.Errorf("finish progress for task %q: %w", name, err)
	}

	return nil
}

func (b *ConsoleProgressBar) Update(name string, percent int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureTaskLocked(name)
	b.percent[name] = clampPercent(percent)

	if err := b.renderLocked(); err != nil {
		return fmt.Errorf("update progress for task %q: %w", name, err)
	}

	return nil
}

func (b *ConsoleProgressBar) Finish(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureTaskLocked(name)
	b.percent[name] = 100
	b.finished[name] = true

	if err := b.renderLocked(); err != nil {
		return fmt.Errorf("finish progress for task %q: %w", name, err)
	}

	return nil
}

func (b *ConsoleProgressBar) ensureTaskLocked(name string) {
	if _, exists := b.percent[name]; exists {
		return
	}

	b.order = append(b.order, name)
	b.percent[name] = 0
	b.finished[name] = false
}

func (b *ConsoleProgressBar) renderLocked() error {
	if len(b.order) == 0 {
		return nil
	}

	if b.rendered && b.ansi {
		if _, err := fmt.Fprintf(b.out, "\033[%dA", len(b.order)); err != nil {
			return &RenderError{
				Op:  "move_cursor",
				Err: err,
			}
		}
	}

	for _, name := range b.order {
		if _, err := fmt.Fprint(b.out, b.formatLine(name)); err != nil {
			return &RenderError{
				Op:   "write_line",
				Task: name,
				Err:  err,
			}
		}
		if _, err := fmt.Fprint(b.out, "\n"); err != nil {
			return &RenderError{
				Op:   "write_newline",
				Task: name,
				Err:  err,
			}
		}
	}

	b.rendered = true

	return nil
}

func (b *ConsoleProgressBar) formatLine(name string) string {
	percent := b.percent[name]

	if percent >= 100 {
		progressBar := "[" + strings.Repeat("=", b.width) + "]"
		if b.ansi {
			return fmt.Sprintf("%-12s %s %sDONE%s", name, progressBar, greenColor, resetColor)
		}
		return fmt.Sprintf("%-12s %s DONE", name, progressBar)
	}

	progressBar := b.punBar(percent)
	return fmt.Sprintf("%-12s %s %3d%%", name, progressBar, percent)
}

func (b *ConsoleProgressBar) punBar(percent int) string {
	cursorPos := b.width * percent / 100
	if cursorPos >= b.width {
		cursorPos = b.width - 1
	}
	if cursorPos < 0 {
		cursorPos = 0
	}

	marker := b.nextMarker()
	left := strings.Repeat("=", cursorPos)
	right := strings.Repeat(" ", b.width-cursorPos-1)

	return "[" + left + string(marker) + right + "]"
}

func (b *ConsoleProgressBar) nextMarker() rune {
	if len(b.markers) == 0 {
		return '#'
	}

	return b.markers[b.random.Intn(len(b.markers))]
}

func clampPercent(percent int) int {
	switch {
	case percent < 0:
		return 0
	case percent > 100:
		return 100
	default:
		return percent
	}
}

func supportsANSI(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}

	if file != os.Stdout && file != os.Stderr {
		return false
	}

	term := strings.TrimSpace(strings.ToLower(os.Getenv("TERM")))
	if term == "" || term == "dumb" {
		return false
	}

	return true
}
