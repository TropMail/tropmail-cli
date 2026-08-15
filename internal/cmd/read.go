package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	tropmail "github.com/tropmail/tropmail-go"
	"golang.org/x/term"

	"github.com/tropmail/tropmail-cli/internal/cache"
	"github.com/tropmail/tropmail-cli/internal/output"
)

// allViews are the cache keys invalidated when an email changes.
var allViews = []tropmail.View{tropmail.ViewText, tropmail.ViewHTML, tropmail.ViewMarkdown}

// fetchDetail returns an email detail, serving the local cache when possible.
//
// Bodies never change once delivered, and the markdown view can take the server
// up to a minute to produce, so a cache hit is the difference between instant
// and slow.
func (a *App) fetchDetail(
	ctx context.Context,
	client *tropmail.Client,
	id string,
	view tropmail.View,
	timestamp string,
) (*tropmail.EmailDetail, bool, error) {
	key := cache.BodyKey(id, string(view))

	var cached tropmail.EmailDetail
	if a.Cache.Get(key, &cached) && cached.Content != "" {
		return &cached, true, nil
	}

	var (
		detail *tropmail.EmailDetail
		err    error
	)
	if view == tropmail.ViewMarkdown {
		detail, err = client.Emails.GetMarkdown(ctx, id, timestamp)
	} else {
		detail, err = client.Emails.Get(ctx, id, tropmail.GetOptions{
			View:      view,
			Timestamp: timestamp,
		})
	}
	if err != nil {
		return nil, false, err
	}

	a.Cache.Put(key, detail)
	return detail, false, nil
}

// invalidate drops every cached view of an email after it changes.
func (a *App) invalidate(id string) {
	for _, view := range allViews {
		a.Cache.Remove(cache.BodyKey(id, string(view)))
	}
}

func newReadCommand() *cobra.Command {
	var (
		view      string
		timestamp string
		raw       bool
		width     int
	)

	cmd := &cobra.Command{
		Use:     "read <email-id>",
		Aliases: []string{"cat", "show"},
		Short:   "Print one email",
		Long: `Print a single email.

The default markdown view is rendered with syntax colours in a terminal and
emitted as plain markdown when piped. Bodies are cached under ~/.cache/tropmail,
so re-reading a message costs no network round trip; pass --no-cache to refetch.`,
		Example: `  tropmail read 018f... 
  tropmail read 018f... --view text
  tropmail read 018f... --raw > message.md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			client, err := app.Client()
			if err != nil {
				return err
			}

			selected := tropmail.View(view)
			switch selected {
			case tropmail.ViewText, tropmail.ViewHTML, tropmail.ViewMarkdown:
			default:
				return fmt.Errorf("unknown view %q: use text, html, or markdown", view)
			}

			detail, cached, err := app.fetchDetail(
				cmd.Context(), client, args[0], selected, timestamp)
			if err != nil {
				return err
			}

			if app.Printer.JSON {
				return app.Printer.PrintJSON(detail)
			}
			if raw {
				fmt.Fprintln(app.Printer.Out, detail.Content)
				return nil
			}

			renderHeader(app.Printer, detail, cached)
			body := detail.Content
			if selected == tropmail.ViewMarkdown && isatty.IsTerminal(os.Stdout.Fd()) {
				body = renderMarkdown(body, width, app.Printer.NoColor)
			}
			fmt.Fprintln(app.Printer.Out, strings.TrimRight(body, "\n"))
			renderAttachments(app.Printer, detail)
			return nil
		},
	}

	cmd.Flags().StringVarP(&view, "view", "v", "markdown", "body view: text, html, or markdown")
	cmd.Flags().StringVar(&timestamp, "timestamp", "",
		"email timestamp from list/detail (speeds up lookup)")
	cmd.Flags().BoolVar(&raw, "raw", false, "print only the body, unstyled")
	cmd.Flags().IntVar(&width, "width", 0, "wrap width for rendered markdown (0 = auto)")
	return cmd
}

func renderHeader(p *printer, detail *tropmail.EmailDetail, cached bool) {
	sender := detail.From.Address
	if detail.From.Name != "" {
		sender = fmt.Sprintf("%s <%s>", detail.From.Name, detail.From.Address)
	}

	p.Printf("%s %s\n", p.Bold("From:   "), sender)
	if recipients := addressList(detail.To); recipients != "" {
		p.Printf("%s %s\n", p.Bold("To:     "), recipients)
	}
	if copies := addressList(detail.Cc); copies != "" {
		p.Printf("%s %s\n", p.Bold("Cc:     "), copies)
	}
	p.Printf("%s %s\n", p.Bold("Subject:"), detail.Subject)
	p.Printf("%s %s\n", p.Bold("Date:   "), detail.Timestamp)

	state := string(detail.EmailState)
	if detail.ActionStatus != nil && *detail.ActionStatus != "" {
		state += " · " + string(*detail.ActionStatus)
	}
	if cached {
		state += " · cached"
	}
	p.Printf("%s\n\n", p.Muted(state))
}

func addressList(addresses []tropmail.Address) string {
	if len(addresses) == 0 {
		return ""
	}
	parts := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.Name != "" {
			parts = append(parts, fmt.Sprintf("%s <%s>", address.Name, address.Address))
			continue
		}
		parts = append(parts, address.Address)
	}
	return strings.Join(parts, ", ")
}

func renderAttachments(p *printer, detail *tropmail.EmailDetail) {
	if len(detail.Attachments) == 0 {
		return
	}
	p.Printf("\n%s\n", p.Bold("Attachments"))
	rows := make([][]string, 0, len(detail.Attachments))
	for _, attachment := range detail.Attachments {
		name := ""
		if attachment.Filename != nil {
			name = *attachment.Filename
		}
		size := ""
		if attachment.Size != nil {
			size = humanSize(*attachment.Size)
		}
		rows = append(rows, []string{
			attachment.AttachmentID,
			output.Truncate(name, 40),
			size,
			string(attachment.ScanStatus),
		})
	}
	p.Table([]string{"ID", "FILE", "SIZE", "SCAN"}, rows)
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGT"[exp])
}

// renderMarkdown styles markdown for the terminal, degrading to the raw text
// when glamour cannot build a renderer.
func renderMarkdown(body string, width int, noColor bool) string {
	options := []glamour.TermRendererOption{glamour.WithWordWrap(wrapWidth(width))}
	if noColor {
		options = append(options, glamour.WithStandardStyle("notty"))
	} else {
		options = append(options, glamour.WithAutoStyle())
	}

	renderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return body
	}
	rendered, err := renderer.Render(body)
	if err != nil {
		return body
	}
	return rendered
}

// wrapWidth keeps prose readable: terminal width, capped so wide windows do not
// produce unreadably long lines.
func wrapWidth(requested int) int {
	if requested > 0 {
		return requested
	}
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 20 {
		return 80
	}
	if width > 100 {
		return 100
	}
	return width - 2
}
