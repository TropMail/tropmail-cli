// Package output renders CLI results as either human tables or machine JSON.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

// Printer writes command results in the format the caller asked for.
type Printer struct {
	JSON    bool
	Quiet   bool
	NoColor bool

	Out io.Writer
	Err io.Writer
}

// Stdout and Stderr are the streams New binds to. Tests replace them to
// capture command output.
var (
	Stdout io.Writer = os.Stdout
	Stderr io.Writer = os.Stderr
)

// New builds a printer bound to the package streams.
func New(asJSON, quiet, noColor bool) *Printer {
	if !noColor {
		noColor = os.Getenv("NO_COLOR") != "" || !isatty.IsTerminal(os.Stdout.Fd())
	}
	return &Printer{
		JSON:    asJSON,
		Quiet:   quiet,
		NoColor: noColor,
		Out:     Stdout,
		Err:     Stderr,
	}
}

// Colors used across the CLI and the TUI.
var (
	ColorAccent = lipgloss.Color("39")
	ColorMuted  = lipgloss.Color("245")
	ColorGood   = lipgloss.Color("42")
	ColorWarn   = lipgloss.Color("214")
	ColorBad    = lipgloss.Color("203")
)

func (p *Printer) style(s lipgloss.Style) lipgloss.Style {
	if p.NoColor {
		return lipgloss.NewStyle()
	}
	return s
}

// Bold renders text in bold unless colour is disabled.
func (p *Printer) Bold(text string) string {
	return p.style(lipgloss.NewStyle().Bold(true)).Render(text)
}

// Muted renders de-emphasised text.
func (p *Printer) Muted(text string) string {
	return p.style(lipgloss.NewStyle().Foreground(ColorMuted)).Render(text)
}

// Accent renders text in the accent colour.
func (p *Printer) Accent(text string) string {
	return p.style(lipgloss.NewStyle().Foreground(ColorAccent)).Render(text)
}

// Print writes a value: JSON when --json is set, otherwise the human renderer.
func (p *Printer) Print(value any, human func(*Printer)) error {
	if p.JSON {
		return p.PrintJSON(value)
	}
	if p.Quiet {
		return nil
	}
	human(p)
	return nil
}

// PrintJSON writes an indented JSON document.
func (p *Printer) PrintJSON(value any) error {
	encoder := json.NewEncoder(p.Out)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

// Printf writes a formatted line to stdout, honouring --quiet.
func (p *Printer) Printf(format string, args ...any) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Out, format, args...)
}

// Warnf writes a formatted line to stderr. Warnings survive --quiet.
func (p *Printer) Warnf(format string, args ...any) {
	fmt.Fprintf(p.Err, format, args...)
}

// Table renders aligned columns, padding each to its widest cell.
func (p *Printer) Table(headers []string, rows [][]string) {
	if p.Quiet {
		return
	}
	if len(rows) == 0 {
		fmt.Fprintln(p.Out, p.Muted("no results"))
		return
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = lipgloss.Width(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && lipgloss.Width(cell) > widths[i] {
				widths[i] = lipgloss.Width(cell)
			}
		}
	}

	var header strings.Builder
	for i, name := range headers {
		header.WriteString(pad(name, widths[i]))
		if i < len(headers)-1 {
			header.WriteString("  ")
		}
	}
	fmt.Fprintln(p.Out, p.Bold(header.String()))

	for _, row := range rows {
		var line strings.Builder
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			line.WriteString(pad(cell, widths[i]))
			if i < len(row)-1 {
				line.WriteString("  ")
			}
		}
		fmt.Fprintln(p.Out, strings.TrimRight(line.String(), " "))
	}
}

func pad(text string, width int) string {
	gap := width - lipgloss.Width(text)
	if gap <= 0 {
		return text
	}
	return text + strings.Repeat(" ", gap)
}

// Truncate shortens text to width, adding an ellipsis when it had to cut.
func Truncate(text string, width int) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "\r", "")
	text = strings.TrimSpace(text)
	if width <= 1 || lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	return string(runes[:width-1]) + "…"
}

// RelativeTime renders an RFC3339 timestamp as a compact age such as "3h".
func RelativeTime(timestamp string) string {
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		if len(timestamp) >= 16 {
			return timestamp[:16]
		}
		return timestamp
	}

	elapsed := time.Since(parsed)
	switch {
	case elapsed < time.Minute:
		return "now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh", int(elapsed.Hours()))
	case elapsed < 365*24*time.Hour:
		return fmt.Sprintf("%dd", int(elapsed.Hours()/24))
	default:
		return parsed.Format("2006-01-02")
	}
}
