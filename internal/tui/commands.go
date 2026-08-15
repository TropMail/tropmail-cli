package tui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	tropmail "github.com/tropmail/tropmail-go"

	"github.com/tropmail/tropmail-cli/internal/cache"
)

// Messages delivered back into Update once background work finishes.
type (
	mailboxLoadedMsg struct {
		mailbox *tropmail.Mailbox
		err     error
	}

	pageLoadedMsg struct {
		page    int
		query   string
		status  tropmail.ListStatus
		list    *tropmail.EmailList
		replace bool
		err     error
	}

	detailLoadedMsg struct {
		id     string
		view   tropmail.View
		detail *tropmail.EmailDetail
		cached bool
		err    error
	}

	actionDoneMsg struct {
		id     string
		label  string
		result *tropmail.ActionResult
		err    error
	}
)

// loadMailbox fetches the mailbox summary shown in the title bar.
func loadMailbox(ctx context.Context, client *tropmail.Client) tea.Cmd {
	return func() tea.Msg {
		mailbox, err := client.Mailbox.Get(ctx)
		return mailboxLoadedMsg{mailbox: mailbox, err: err}
	}
}

// loadPage fetches one page of the inbox or of a search, off the UI thread.
func loadPage(
	ctx context.Context,
	client *tropmail.Client,
	query string,
	status tropmail.ListStatus,
	page, pageSize int,
	replace bool,
) tea.Cmd {
	return func() tea.Msg {
		opts := tropmail.ListOptions{Limit: pageSize, Page: page, Status: status}

		var (
			list *tropmail.EmailList
			err  error
		)
		if query == "" {
			list, err = client.Emails.List(ctx, opts)
		} else {
			list, err = client.Emails.Search(ctx, query, opts)
		}
		return pageLoadedMsg{
			page:    page,
			query:   query,
			status:  status,
			list:    list,
			replace: replace,
			err:     err,
		}
	}
}

// loadDetail fetches a body, serving the on-disk cache when it already has one.
func loadDetail(
	ctx context.Context,
	client *tropmail.Client,
	store *cache.Cache,
	email tropmail.Email,
	view tropmail.View,
) tea.Cmd {
	return func() tea.Msg {
		key := cache.BodyKey(email.ID, string(view))

		var cached tropmail.EmailDetail
		if store.Get(key, &cached) && cached.Content != "" {
			return detailLoadedMsg{id: email.ID, view: view, detail: &cached, cached: true}
		}

		var (
			detail *tropmail.EmailDetail
			err    error
		)
		if view == tropmail.ViewMarkdown {
			detail, err = client.Emails.GetMarkdown(ctx, email.ID, email.Timestamp)
		} else {
			detail, err = client.Emails.Get(ctx, email.ID, tropmail.GetOptions{
				View:      view,
				Timestamp: email.Timestamp,
			})
		}
		if err != nil {
			return detailLoadedMsg{id: email.ID, view: view, err: err}
		}

		store.Put(key, detail)
		return detailLoadedMsg{id: email.ID, view: view, detail: detail}
	}
}

// copyToClipboard sets the system clipboard with an OSC 52 escape, which works
// over SSH and in multiplexers. Terminals that do not support it ignore the
// sequence, so a failure is silent by design.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		encoded := base64.StdEncoding.EncodeToString([]byte(text))
		// One Write so the sequence cannot interleave with a rendered frame.
		fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", encoded)
		return nil
	}
}

// applyAction runs a state or status change and reports the outcome.
func applyAction(
	ctx context.Context,
	client *tropmail.Client,
	store *cache.Cache,
	email tropmail.Email,
	label string,
	run func(context.Context, *tropmail.Client, string) (*tropmail.ActionResult, error),
) tea.Cmd {
	return func() tea.Msg {
		result, err := run(ctx, client, email.ID)
		if err == nil {
			for _, view := range []tropmail.View{
				tropmail.ViewText, tropmail.ViewHTML, tropmail.ViewMarkdown,
			} {
				store.Remove(cache.BodyKey(email.ID, string(view)))
			}
		}
		return actionDoneMsg{id: email.ID, label: label, result: result, err: err}
	}
}
