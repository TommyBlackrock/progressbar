package progressbar

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewConsoleProgressBarUsesDefaultWidth(t *testing.T) {
	bar := NewConsoleProgressBarWithWriter(0, &bytes.Buffer{})

	require.Equal(t, defaultBarWidth, bar.width)
	require.False(t, bar.ansi)
}

func TestClampPercent(t *testing.T) {
	require.Equal(t, 0, clampPercent(-10))
	require.Equal(t, 0, clampPercent(0))
	require.Equal(t, 55, clampPercent(55))
	require.Equal(t, 100, clampPercent(100))
	require.Equal(t, 100, clampPercent(120))
}

func TestFormatLineInProgress(t *testing.T) {
	bar := NewConsoleProgressBarWithWriter(10, &bytes.Buffer{})
	bar.random = rand.New(rand.NewSource(1))
	bar.Add("worker")
	bar.percent["worker"] = 40

	line := bar.formatLine("worker")

	require.Regexp(t, `^worker\s+\[====[#abcdefjklmnopqrxyz]{1}[ ]{5}\]\s+40%$`, line)
}

func TestFormatLineDoneWithoutANSI(t *testing.T) {
	bar := NewConsoleProgressBarWithWriter(10, &bytes.Buffer{})
	bar.Add("worker")
	bar.percent["worker"] = 100

	line := bar.formatLine("worker")

	require.Equal(t, "worker       [==========] DONE", line)
	require.NotContains(t, line, "\033[32m")
}

func TestRenderWithoutANSIDoesNotEmitEscapeCodes(t *testing.T) {
	var out bytes.Buffer
	bar := NewConsoleProgressBarWithWriter(10, &out)
	bar.random = rand.New(rand.NewSource(1))

	require.NoError(t, bar.Update("worker", 40))
	require.NoError(t, bar.Update("worker", 80))
	require.NoError(t, bar.Finish("worker"))

	rendered := out.String()
	require.NotContains(t, rendered, "\033[")
	require.Regexp(t, regexp.MustCompile(`worker\s+\[====[#abcdefjklmnopqrxyz]{1}[ ]{5}\]\s+40%`), rendered)
	require.Regexp(t, regexp.MustCompile(`worker\s+\[========[#abcdefjklmnopqrxyz]{1}[ ]{1}\]\s+80%`), rendered)
	require.Contains(t, rendered, "worker       [==========] DONE")
}

func TestRenderWithANSIRewritesPreviousLines(t *testing.T) {
	var out bytes.Buffer
	bar := NewConsoleProgressBarWithWriter(5, &out)
	bar.ansi = true
	bar.random = rand.New(rand.NewSource(1))

	require.NoError(t, bar.Update("one", 20))
	require.NoError(t, bar.Update("two", 40))
	require.NoError(t, bar.Update("one", 60))

	rendered := out.String()
	require.Contains(t, rendered, "\033[2A")
	require.Regexp(t, regexp.MustCompile(`one\s+\[===[#abcdefjklmnopqrxyz]{1}[ ]{1}\]\s+60%`), rendered)
	require.Regexp(t, regexp.MustCompile(`two\s+\[==[#abcdefjklmnopqrxyz]{1}[ ]{2}\]\s+40%`), rendered)
}

func TestRunMarksTaskFinished(t *testing.T) {
	var out bytes.Buffer
	bar := NewConsoleProgressBarWithWriter(10, &out)

	progress := make(chan int, 3)
	progress <- 10
	progress <- 90
	progress <- 100
	close(progress)

	require.NoError(t, bar.Run("job", progress))

	require.True(t, bar.finished["job"])
	require.Equal(t, 100, bar.percent["job"])
	require.Contains(t, out.String(), "job          [==========] DONE")
}

func TestAddDoesNotDuplicateTaskOrder(t *testing.T) {
	bar := NewConsoleProgressBarWithWriter(10, &bytes.Buffer{})

	bar.Add("job")
	bar.Add("job")

	require.Equal(t, []string{"job"}, bar.order)
}

func TestSupportsANSIForNonFileWriterIsFalse(t *testing.T) {
	require.False(t, supportsANSI(&bytes.Buffer{}))
}

func TestRenderedOutputHasExpectedLineCountWithoutANSI(t *testing.T) {
	var out bytes.Buffer
	bar := NewConsoleProgressBarWithWriter(10, &out)

	require.NoError(t, bar.Update("first", 10))
	require.NoError(t, bar.Update("second", 20))
	require.NoError(t, bar.Update("first", 30))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 5)
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

func TestUpdateReturnsWrappedWriterError(t *testing.T) {
	bar := NewConsoleProgressBarWithWriter(10, failingWriter{err: io.ErrClosedPipe})

	err := bar.Update("job", 10)
	require.Error(t, err)
	require.True(t, errors.Is(err, io.ErrClosedPipe))

	var renderErr *RenderError
	require.True(t, errors.As(err, &renderErr))
	require.Equal(t, "write_line", renderErr.Op)
	require.Equal(t, "job", renderErr.Task)
}
