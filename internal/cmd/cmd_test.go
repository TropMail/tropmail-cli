package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tropmail/tropmail-cli/internal/output"
)

const testAPIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var emailPayload = map[string]any{
	"id":               "11111111-1111-1111-1111-111111111111",
	"timestamp":        "2026-01-01T00:00:00Z",
	"subject":          "Welcome to TropMail",
	"from":             map[string]any{"name": "Sender", "address": "sender@example.com"},
	"preview":          "Preview text",
	"attachmentsCount": 1,
	"status":           "Open",
	"email_state":      "Open",
}

var detailPayload = map[string]any{
	"id":          "11111111-1111-1111-1111-111111111111",
	"timestamp":   "2026-01-01T00:00:00Z",
	"subject":     "Welcome to TropMail",
	"from":        map[string]any{"name": "Sender", "address": "sender@example.com"},
	"to":          []any{map[string]any{"name": "", "address": "me@tropmail.com"}},
	"cc":          []any{},
	"content":     "# Heading\n\nHello there.",
	"status":      "Open",
	"email_state": "Open",
	"attachments": []any{
		map[string]any{
			"attachment_id": "22222222-2222-2222-2222-222222222222",
			"filename":      "invoice.pdf",
			"size":          2048,
			"mime_type":     "application/pdf",
			"scan_status":   "Clean",
		},
	},
	"headers":  map[string]any{},
	"security": map[string]any{},
}

var mailboxPayload = map[string]any{
	"id": "mb1", "email": "user@tropmail.com",
	"opened_count": 3, "closed_count": 1, "favorite_count": 2,
}

// fakeAPI serves the envelope for every route the CLI touches.
func fakeAPI(t *testing.T, requests *[]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			*requests = append(*requests, r.Method+" "+r.URL.Path)
		}

		var data any
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		switch {
		case path == "/mailboxes":
			data = map[string]any{"mailboxes": []any{mailboxPayload}}
		case path == "/health":
			data = map[string]any{"status": "ok", "version": "1.0.0", "timestamp": "t"}
		case strings.HasSuffix(path, "/scan-attachments"),
			strings.HasSuffix(path, "/download-attachments"):
			data = []any{}
		case strings.HasSuffix(path, "/download"):
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="invoice.pdf"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("%PDF-1.4 mock"))
			return
		case strings.Contains(path, "/emails/search"):
			data = map[string]any{
				"emails": []any{emailPayload}, "total": 0, "limit": 20, "page": 1,
			}
		case strings.HasSuffix(path, "/emails"):
			data = map[string]any{
				"emails": []any{emailPayload}, "total": 4, "limit": 20, "page": 1,
			}
		case strings.Contains(path, "/emails/") && r.Method == http.MethodPost:
			data = map[string]any{"email_id": "11111111", "action_status": "Favorite"}
		case strings.Contains(path, "/emails/"):
			data = detailPayload
		case strings.HasSuffix(path, "/scan"):
			data = map[string]any{
				"attachment_id": "22222222", "filename": "invoice.pdf",
				"scan_status": "Processing", "status": "Processing",
			}
		case strings.Contains(path, "/attachments/"):
			data = map[string]any{
				"attachment_id": "22222222", "email_id": "11111111",
				"filename": "invoice.pdf", "size": 2048, "scan_status": "Clean",
			}
		case strings.HasPrefix(path, "/mailboxes/"):
			data = mailboxPayload
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "3")
		w.Header().Set("X-RateLimit-Remaining", "2")
		w.Header().Set("X-RateLimit-Reset", "1767225600")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "message": "ok", "data": data, "error": nil,
		})
	}))
	t.Cleanup(server.Close)
	return server
}

// capture redirects the printer streams for the duration of a test.
func capture(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	output.Stdout, output.Stderr = &stdout, &stderr
	t.Cleanup(func() { output.Stdout, output.Stderr = os.Stdout, os.Stderr })
	return &stdout, &stderr
}

// isolate points config and credentials at a throwaway directory.
func isolate(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("TROPMAIL_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("TROPMAIL_API_KEY", testAPIKey)
	t.Setenv("NO_COLOR", "1")
}

// run executes the CLI in-process and returns stdout, stderr, and the error.
func run(t *testing.T, serverURL string, args ...string) (string, string, error) {
	t.Helper()

	isolate(t)
	stdout, stderr := capture(t)

	root := newRootCommand()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(append(args, "--base-url", serverURL+"/api/v1"))

	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func decode(t *testing.T, payload string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, payload)
	}
	return out
}

func TestMailboxCommand(t *testing.T) {
	server := fakeAPI(t, nil)

	stdout, _, err := run(t, server.URL, "mailbox", "--json")
	if err != nil {
		t.Fatalf("mailbox: %v", err)
	}
	if got := decode(t, stdout)["email"]; got != "user@tropmail.com" {
		t.Errorf("email = %v", got)
	}
}

func TestListCommandJSON(t *testing.T) {
	server := fakeAPI(t, nil)

	stdout, _, err := run(t, server.URL, "ls", "--json", "--limit", "20")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	payload := decode(t, stdout)
	emails, ok := payload["emails"].([]any)
	if !ok || len(emails) != 1 {
		t.Fatalf("emails = %v", payload["emails"])
	}
	first := emails[0].(map[string]any)
	if first["subject"] != "Welcome to TropMail" {
		t.Errorf("subject = %v", first["subject"])
	}
}

func TestListCommandTable(t *testing.T) {
	server := fakeAPI(t, nil)

	stdout, _, err := run(t, server.URL, "ls")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	for _, want := range []string{"SUBJECT", "Welcome to TropMail", "in mailbox"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %q:\n%s", want, stdout)
		}
	}
}

func TestSearchCommand(t *testing.T) {
	server := fakeAPI(t, nil)

	stdout, _, err := run(t, server.URL, "search", "welcome", "--json")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if _, ok := decode(t, stdout)["emails"]; !ok {
		t.Errorf("missing emails key:\n%s", stdout)
	}
}

func TestReadCommandFetchesBodyEachTime(t *testing.T) {
	var requests []string
	server := fakeAPI(t, &requests)

	isolate(t)
	stdout, stderr := capture(t)

	for i := 0; i < 2; i++ {
		stdout.Reset()
		root := newRootCommand()
		root.SetArgs([]string{
			"read", "11111111-1111-1111-1111-111111111111",
			"--view", "text", "--base-url", server.URL + "/api/v1",
		})
		if err := root.Execute(); err != nil {
			t.Fatalf("read %d: %v (%s)", i, err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Hello there.") {
			t.Errorf("run %d missing body:\n%s", i, stdout.String())
		}
	}

	detailCalls := 0
	for _, request := range requests {
		if strings.Contains(request, "/emails/11111111") {
			detailCalls++
		}
	}
	if detailCalls != 2 {
		t.Errorf("fetched the body %d times, want 2 (bodies are not stored on disk)",
			detailCalls)
	}
}

func TestReadRejectsUnknownView(t *testing.T) {
	server := fakeAPI(t, nil)

	if _, _, err := run(t, server.URL, "read", "abc", "--view", "pdf"); err == nil {
		t.Fatal("expected an error for an unknown view")
	}
}

func TestActionCommand(t *testing.T) {
	var requests []string
	server := fakeAPI(t, &requests)

	stdout, _, err := run(t, server.URL, "fav", "11111111-1111-1111-1111-111111111111", "--json")
	if err != nil {
		t.Fatalf("fav: %v", err)
	}
	if decode(t, stdout)["action"] != "fav" {
		t.Errorf("unexpected payload: %s", stdout)
	}

	found := false
	for _, request := range requests {
		if strings.HasPrefix(request, "POST /api/v1/mailboxes/") {
			found = true
		}
	}
	if !found {
		t.Errorf("no action POST recorded: %v", requests)
	}
}

func TestAttachCommands(t *testing.T) {
	server := fakeAPI(t, nil)

	stdout, _, err := run(t, server.URL, "attach", "info", "22222222", "--json")
	if err != nil {
		t.Fatalf("attach info: %v", err)
	}
	if decode(t, stdout)["filename"] != "invoice.pdf" {
		t.Errorf("unexpected payload: %s", stdout)
	}

	stdout, _, err = run(t, server.URL, "attach", "ls",
		"11111111-1111-1111-1111-111111111111", "--json")
	if err != nil {
		t.Fatalf("attach ls: %v", err)
	}
	if decode(t, stdout)["count"].(float64) != 1 {
		t.Errorf("unexpected count: %s", stdout)
	}
}

func TestAuthStatusCommand(t *testing.T) {
	server := fakeAPI(t, nil)

	stdout, _, err := run(t, server.URL, "auth", "status", "--json")
	if err != nil {
		t.Fatalf("auth status: %v", err)
	}
	payload := decode(t, stdout)
	if payload["storage"] != "environment" {
		t.Errorf("unexpected payload: %s", stdout)
	}
}

func TestWatchBaselineExits(t *testing.T) {
	server := fakeAPI(t, nil)

	if _, _, err := run(t, server.URL, "watch", "--once"); err != nil {
		t.Fatalf("watch --once: %v", err)
	}
}

func TestUnknownRouteSurfacesPlainTextError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	t.Cleanup(server.Close)

	_, _, err := run(t, server.URL, "mailbox")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("error = %v", err)
	}
	if code := exitCodeFor(err); code != ExitNotFound {
		t.Errorf("exit code = %d, want %d", code, ExitNotFound)
	}
}

func TestVersionCommandNeedsNoCredentials(t *testing.T) {
	t.Setenv("TROPMAIL_API_KEY", "")
	stdout, _ := capture(t)

	root := newRootCommand()
	root.SetArgs([]string{"version", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if decode(t, stdout.String())["version"] == "" {
		t.Errorf("missing version: %s", stdout.String())
	}
}
