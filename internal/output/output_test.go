package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testPrinter(asJSON, quiet bool) (*Printer, *bytes.Buffer, *bytes.Buffer) {
	var out, err bytes.Buffer
	return &Printer{JSON: asJSON, Quiet: quiet, NoColor: true, Out: &out, Err: &err}, &out, &err
}

func TestPrintUsesJSONWhenRequested(t *testing.T) {
	printer, out, _ := testPrinter(true, false)

	humanCalled := false
	err := printer.Print(map[string]string{"a": "b"}, func(*Printer) { humanCalled = true })
	if err != nil {
		t.Fatalf("print: %v", err)
	}
	if humanCalled {
		t.Error("the human renderer ran despite --json")
	}

	var decoded map[string]string
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if decoded["a"] != "b" {
		t.Errorf("decoded = %v", decoded)
	}
}

func TestQuietSuppressesHumanOutputButNotJSON(t *testing.T) {
	printer, out, _ := testPrinter(false, true)
	_ = printer.Print("value", func(p *Printer) { p.Printf("visible\n") })
	if out.Len() != 0 {
		t.Errorf("quiet printed %q", out.String())
	}

	printer, out, _ = testPrinter(true, true)
	_ = printer.Print("value", func(*Printer) {})
	if out.Len() == 0 {
		t.Error("--quiet must not suppress --json")
	}
}

func TestWarningsSurviveQuiet(t *testing.T) {
	printer, _, errBuf := testPrinter(false, true)
	printer.Warnf("careful\n")
	if !strings.Contains(errBuf.String(), "careful") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestTableAligns(t *testing.T) {
	printer, out, _ := testPrinter(false, false)
	printer.Table([]string{"ID", "NAME"}, [][]string{
		{"1", "short"},
		{"22", "a longer value"},
	})

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d:\n%s", len(lines), out.String())
	}
	// Both data rows should start their second column at the same offset.
	if strings.Index(lines[1], "short") != strings.Index(lines[2], "a longer") {
		t.Errorf("columns are not aligned:\n%s", out.String())
	}
}

func TestEmptyTableSaysSo(t *testing.T) {
	printer, out, _ := testPrinter(false, false)
	printer.Table([]string{"ID"}, nil)
	if !strings.Contains(out.String(), "no results") {
		t.Errorf("output = %q", out.String())
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{"short text is untouched", "hello", 10, "hello"},
		{"long text gets an ellipsis", "hello world", 8, "hello w…"},
		{"newlines collapse to spaces", "line\nbreak", 20, "line break"},
		{"exact width is untouched", "12345", 5, "12345"},
		{"multibyte is cut by rune", "héllo wörld", 8, "héllo w…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Truncate(tc.text, tc.width); got != tc.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tc.text, tc.width, got, tc.want)
			}
		})
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name  string
		stamp string
		want  string
	}{
		{"seconds", now.Add(-10 * time.Second).Format(time.RFC3339), "now"},
		{"minutes", now.Add(-30 * time.Minute).Format(time.RFC3339), "30m"},
		{"hours", now.Add(-5 * time.Hour).Format(time.RFC3339), "5h"},
		{"days", now.Add(-72 * time.Hour).Format(time.RFC3339), "3d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelativeTime(tc.stamp); got != tc.want {
				t.Errorf("RelativeTime(%q) = %q, want %q", tc.stamp, got, tc.want)
			}
		})
	}
}

func TestRelativeTimeTolerAtesUnparsableInput(t *testing.T) {
	if got := RelativeTime("2026-01-01 00:00:00+00"); got != "2026-01-01 00:00" {
		t.Errorf("got %q", got)
	}
	if got := RelativeTime("nope"); got != "nope" {
		t.Errorf("got %q", got)
	}
}
