package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	tropmail "github.com/tropmail/tropmail-go"

	"github.com/tropmail/tropmail-cli/internal/cache"
)

const apiKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// stubAPI answers every call with a valid envelope so background commands can
// run without reaching the network.
func stubAPI(t *testing.T) *tropmail.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		switch {
		case strings.HasSuffix(r.URL.Path, "/mailbox"):
			data = map[string]any{
				"id": "mb1", "email": "user@tropmail.com",
				"opened_count": 2, "closed_count": 1, "favorite_count": 1,
			}
		case strings.Contains(r.URL.Path, "/email/"):
			data = map[string]any{
				"id": "a1", "subject": "One", "content": "body",
				"from":        map[string]any{"name": "", "address": "s@example.com"},
				"email_state": "Open", "timestamp": "2026-01-01T00:00:00Z",
			}
		default:
			data = map[string]any{"emails": []any{}, "total": 0, "limit": 50, "page": 1}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "message": "ok", "data": data,
		})
	}))
	t.Cleanup(server.Close)

	client, err := tropmail.New(apiKey, tropmail.WithBaseURL(server.URL+"/api/v1"))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return client
}

func testModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("TROPMAIL_CACHE_DIR", t.TempDir())

	model := newModel(context.Background(), Options{
		Client:   stubAPI(t),
		Cache:    cache.New(true),
		Status:   tropmail.StatusAll,
		PageSize: 50,
		NoColor:  true,
	})
	next, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(Model)
}

func fixtures(count int) []tropmail.Email {
	emails := make([]tropmail.Email, 0, count)
	for i := 0; i < count; i++ {
		emails = append(emails, tropmail.Email{
			ID:         fmt.Sprintf("id-%02d", i),
			Subject:    fmt.Sprintf("Subject %02d", i),
			From:       tropmail.Address{Name: fmt.Sprintf("Sender %02d", i), Address: "s@example.com"},
			Timestamp:  "2026-01-01T00:00:00Z",
			EmailState: tropmail.StateOpen,
		})
	}
	return emails
}

// load feeds a page of results into the model the way the loader command would.
func load(t *testing.T, model Model, emails []tropmail.Email, page int, replace bool) Model {
	t.Helper()

	next, _ := model.Update(pageLoadedMsg{
		page:    page,
		query:   model.query,
		status:  model.status,
		list:    &tropmail.EmailList{Emails: emails, Page: page},
		replace: replace,
	})
	return next.(Model)
}

func press(t *testing.T, model Model, keys string) Model {
	t.Helper()

	for _, r := range keys {
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = next.(Model)
	}
	return model
}

func TestListRendersLoadedEmails(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	view := model.View()
	for _, want := range []string{"Sender 00", "Subject 00", "Sender 02"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestCursorMovesWithVimKeys(t *testing.T) {
	model := load(t, testModel(t), fixtures(5), 1, true)

	model = press(t, model, "jj")
	if model.cursor != 2 {
		t.Errorf("cursor = %d, want 2", model.cursor)
	}
	model = press(t, model, "k")
	if model.cursor != 1 {
		t.Errorf("cursor = %d, want 1", model.cursor)
	}
	model = press(t, model, "G")
	if model.cursor != 4 {
		t.Errorf("cursor = %d, want 4", model.cursor)
	}
	model = press(t, model, "g")
	if model.cursor != 0 {
		t.Errorf("cursor = %d, want 0", model.cursor)
	}
}

func TestCursorStopsAtTheEnds(t *testing.T) {
	model := load(t, testModel(t), fixtures(2), 1, true)

	model = press(t, model, "kkk")
	if model.cursor != 0 {
		t.Errorf("cursor went above the first row: %d", model.cursor)
	}
	model = press(t, model, "jjjjj")
	if model.cursor != 1 {
		t.Errorf("cursor went past the last row: %d", model.cursor)
	}
}

func TestListPaneFitsItsHeight(t *testing.T) {
	model := load(t, testModel(t), fixtures(100), 1, true)

	// Two lines per email, so the pane must not render more rows than fit.
	lines := strings.Count(model.listView(), "\n") + 1
	if lines > model.bodyHeight() {
		t.Errorf("list rendered %d lines into a %d-line pane", lines, model.bodyHeight())
	}
}

func TestWholeViewFitsTheWindow(t *testing.T) {
	model := load(t, testModel(t), fixtures(100), 1, true)

	lines := strings.Count(model.View(), "\n") + 1
	if lines > model.height {
		t.Errorf("view is %d lines tall in a %d-line window", lines, model.height)
	}
}

func TestPrefetchTriggersNearTheEnd(t *testing.T) {
	model := testModel(t)
	model.pageSize = 10
	model = load(t, model, fixtures(10), 1, true)

	if !model.hasMore {
		t.Fatal("a full page should imply there is more")
	}

	// Walk to within the prefetch margin of the end.
	model = press(t, model, strings.Repeat("j", 10-prefetchMargin))
	if !model.loadingPage {
		t.Error("nearing the end did not start the next page")
	}
}

func TestShortPageEndsPagination(t *testing.T) {
	model := testModel(t)
	model.pageSize = 10
	model = load(t, model, fixtures(4), 1, true)

	if model.hasMore {
		t.Error("a short page must end pagination; total is unreliable on search")
	}
	model = press(t, model, "jjj")
	if model.loadingPage {
		t.Error("prefetched past the end of the mailbox")
	}
}

func TestSecondPageAppends(t *testing.T) {
	model := testModel(t)
	model.pageSize = 10
	model = load(t, model, fixtures(10), 1, true)
	model.cursor = 9

	second := make([]tropmail.Email, 0, 3)
	for i, email := range fixtures(3) {
		email.ID = fmt.Sprintf("page2-%d", i)
		second = append(second, email)
	}
	model = load(t, model, second, 2, false)

	if len(model.emails) != 13 {
		t.Errorf("emails = %d, want 13", len(model.emails))
	}
	if model.cursor != 9 {
		t.Errorf("appending moved the cursor to %d", model.cursor)
	}
}

func TestStalePageIsDiscarded(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	// A response for a filter the user has since changed must not land.
	next, _ := model.Update(pageLoadedMsg{
		page:    1,
		query:   "an old search",
		status:  model.status,
		list:    &tropmail.EmailList{Emails: fixtures(9)},
		replace: true,
	})
	if got := len(next.(Model).emails); got != 3 {
		t.Errorf("emails = %d, want the original 3", got)
	}
}

func TestSearchModeCapturesTyping(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = next.(Model)
	if model.mode != modeSearch {
		t.Fatal("'/' did not open search")
	}

	// Keys that are actions in browse mode must reach the text input instead.
	model = press(t, model, "fdq")
	if got := model.input.Value(); got != "fdq" {
		t.Errorf("input = %q, want %q", got, "fdq")
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.mode != modeBrowse || model.query != "fdq" {
		t.Errorf("mode = %v, query = %q", model.mode, model.query)
	}
	if len(model.emails) != 0 || !model.loadingPage {
		t.Error("submitting a search should clear the list and load page 1")
	}
}

func TestEscapeAbandonsSearch(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = press(t, next.(Model), "abc")
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(Model)

	if model.mode != modeBrowse {
		t.Error("escape did not leave search mode")
	}
	if model.query != "" || len(model.emails) != 3 {
		t.Error("escape should not have applied the search")
	}
}

func TestFilterModeSetsStatus(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = press(t, next.(Model), "Favorite")
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)

	if model.status != tropmail.ListStatus("Favorite") {
		t.Errorf("status = %q", model.status)
	}
	if !strings.Contains(model.View(), "filter: Favorite") {
		t.Errorf("the title bar does not show the filter:\n%s", model.View())
	}
}

func TestActionUpdatesTheRowWithoutRefetching(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	favorite := tropmail.ActionFavorite
	next, _ := model.Update(actionDoneMsg{
		id:     "id-00",
		label:  "favorited",
		result: &tropmail.ActionResult{EmailState: tropmail.StateClose, ActionStatus: &favorite},
	})
	model = next.(Model)

	if model.emails[0].EmailState != tropmail.StateClose {
		t.Errorf("state = %q", model.emails[0].EmailState)
	}
	if model.emails[0].ActionStatus == nil || *model.emails[0].ActionStatus != favorite {
		t.Errorf("action status = %v", model.emails[0].ActionStatus)
	}
	if !strings.Contains(model.View(), "★") {
		t.Errorf("the favorite mark is missing from the row:\n%s", model.View())
	}
}

func TestActionKeyShowsProgress(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	model = press(t, model, "b")
	if model.message != "blocking…" {
		t.Errorf("message = %q, want %q", model.message, "blocking…")
	}
}

func TestActionFailureIsReportedNotFatal(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	next, _ := model.Update(actionDoneMsg{
		id: "id-00", label: "blocked", err: fmt.Errorf("boom"),
	})
	model = next.(Model)

	if !model.isError || !strings.Contains(model.message, "boom") {
		t.Errorf("message = %q, isError = %v", model.message, model.isError)
	}
	if model.fatal != nil {
		t.Error("a failed action must not be fatal")
	}
}

func TestFirstPageFailureQuits(t *testing.T) {
	model := testModel(t)

	next, cmd := model.Update(pageLoadedMsg{
		page: 1, query: model.query, status: model.status,
		replace: true, err: fmt.Errorf("unauthorized"),
	})
	if next.(Model).fatal == nil {
		t.Error("a failure with nothing on screen should be fatal")
	}
	if cmd == nil {
		t.Error("expected a quit command")
	}
}

func TestLaterPageFailureIsNotFatal(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	next, _ := model.Update(pageLoadedMsg{
		page: 2, query: model.query, status: model.status, err: fmt.Errorf("boom"),
	})
	model = next.(Model)

	if model.fatal != nil {
		t.Error("a page failure with results on screen must not be fatal")
	}
	if !model.isError {
		t.Error("the failure was not reported")
	}
}

func TestDetailForAnotherRowIsIgnored(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	next, _ := model.Update(detailLoadedMsg{
		id:     "id-02",
		detail: &tropmail.EmailDetail{ID: "id-02", Content: "late body"},
	})
	if next.(Model).detail != nil {
		t.Error("a body for a row the user has scrolled past was applied")
	}
}

func TestPreviewRendersTheDetail(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	next, _ := model.Update(detailLoadedMsg{
		id: "id-00",
		detail: &tropmail.EmailDetail{
			ID: "id-00", Subject: "Subject 00", Content: "# Hello\n\nWorld.",
			From:       tropmail.Address{Address: "s@example.com"},
			EmailState: tropmail.StateOpen,
		},
	})
	model = next.(Model)

	if !strings.Contains(model.viewport.View(), "Hello") {
		t.Errorf("preview is missing the body:\n%s", model.viewport.View())
	}
}

func TestAttachmentsPaneToggles(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	name := "invoice.pdf"
	size := int64(2048)
	next, _ := model.Update(detailLoadedMsg{
		id: "id-00",
		detail: &tropmail.EmailDetail{
			ID: "id-00", Content: "body",
			Attachments: []tropmail.EmailAttachment{{
				AttachmentID: "att-1", Filename: &name, Size: &size,
				ScanStatus: tropmail.ScanClean,
			}},
		},
	})
	model = press(t, next.(Model), "a")

	preview := model.viewport.View()
	if !strings.Contains(preview, "invoice.pdf") || !strings.Contains(preview, "2.0 KB") {
		t.Errorf("attachment pane:\n%s", preview)
	}

	model = press(t, model, "a")
	if !strings.Contains(model.viewport.View(), "body") {
		t.Error("toggling attachments off did not restore the body")
	}
}

func TestViewCycles(t *testing.T) {
	model := load(t, testModel(t), fixtures(1), 1, true)

	if model.view != tropmail.ViewMarkdown {
		t.Fatalf("initial view = %q", model.view)
	}
	model = press(t, model, "v")
	if model.view != tropmail.ViewText {
		t.Errorf("view = %q, want text", model.view)
	}
	model = press(t, model, "v")
	if model.view != tropmail.ViewHTML {
		t.Errorf("view = %q, want html", model.view)
	}
	model = press(t, model, "v")
	if model.view != tropmail.ViewMarkdown {
		t.Errorf("view = %q, want markdown", model.view)
	}
}

func TestTabSwitchesPanes(t *testing.T) {
	model := load(t, testModel(t), fixtures(3), 1, true)

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = next.(Model)
	if model.focus != focusPreview {
		t.Error("tab did not focus the preview")
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if next.(Model).focus != focusList {
		t.Error("tab did not return focus to the list")
	}
}

func TestQuitReturnsTheQuitCommand(t *testing.T) {
	model := load(t, testModel(t), fixtures(1), 1, true)

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q produced no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q did not quit")
	}
}

func TestEmptyMailboxRenders(t *testing.T) {
	model := load(t, testModel(t), nil, 1, true)

	if !strings.Contains(model.View(), "no messages") {
		t.Errorf("view:\n%s", model.View())
	}
	// Keys must not panic with nothing selected.
	press(t, model, "jkfbdvay")
}

func TestNarrowWindowStillRenders(t *testing.T) {
	model := testModel(t)
	next, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	model = load(t, next.(Model), fixtures(5), 1, true)

	if model.View() == "" {
		t.Error("a narrow window rendered nothing")
	}
	lines := strings.Count(model.View(), "\n") + 1
	if lines > model.height {
		t.Errorf("view is %d lines tall in a %d-line window", lines, model.height)
	}
}

func TestMailboxSummaryReachesTheTitle(t *testing.T) {
	model := load(t, testModel(t), fixtures(1), 1, true)

	next, _ := model.Update(mailboxLoadedMsg{mailbox: &tropmail.Mailbox{
		Email: "user@tropmail.com", OpenedCount: 2, ClosedCount: 1, FavoriteCount: 1,
	}})
	if !strings.Contains(next.(Model).View(), "user@tropmail.com") {
		t.Errorf("title:\n%s", next.(Model).View())
	}
}
