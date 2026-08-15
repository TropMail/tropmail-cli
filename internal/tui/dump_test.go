package tui

import (
	"os"
	"testing"

	tropmail "github.com/tropmail/tropmail-go"
)

// TestDumpView is a visual aid: run with -v to print a rendered frame.
func TestDumpView(t *testing.T) {
	if os.Getenv("TROPMAIL_DUMP") == "" {
		t.Skip("set TROPMAIL_DUMP=1 to print a frame")
	}

	model := load(t, testModel(t), fixtures(12), 1, true)
	favorite := tropmail.ActionFavorite
	model.emails[1].ActionStatus = &favorite
	model.emails[2].AttachmentsCount = 2
	model.cursor = 1

	next, _ := model.Update(mailboxLoadedMsg{mailbox: &tropmail.Mailbox{
		Email: "quiet-otter-1423@tropmail.com", OpenedCount: 8, ClosedCount: 4, FavoriteCount: 1,
	}})
	model = next.(Model)

	next, _ = model.Update(detailLoadedMsg{
		id: "id-01",
		detail: &tropmail.EmailDetail{
			ID: "id-01", Subject: "Subject 01", Timestamp: "2026-01-01T00:00:00Z",
			From:       tropmail.Address{Name: "Sender 01", Address: "sender@example.com"},
			To:         []tropmail.Address{{Address: "quiet-otter-1423@tropmail.com"}},
			EmailState: tropmail.StateOpen,
			Content:    "# Your receipt\n\nThanks for your order.\n\n- One widget\n- Two gadgets\n",
		},
	})

	t.Log("\n" + next.(Model).View())
}
