package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	tropmail "github.com/tropmail/tropmail-go"

	"github.com/tropmail/tropmail-cli/internal/output"
)

// emailTable renders a list of emails as aligned columns.
func emailTable(p *printer, emails []tropmail.Email) {
	rows := make([][]string, 0, len(emails))
	for _, email := range emails {
		sender := email.From.Address
		if email.From.Name != "" {
			sender = email.From.Name
		}
		flags := ""
		if email.EmailState == tropmail.StateOpen {
			flags += "o"
		}
		if email.ActionStatus != nil && *email.ActionStatus != "" {
			flags += string(*email.ActionStatus)[:1]
		}
		if email.AttachmentsCount > 0 {
			flags += fmt.Sprintf("@%d", email.AttachmentsCount)
		}
		rows = append(rows, []string{
			email.ID,
			output.RelativeTime(email.Timestamp),
			output.Truncate(sender, 28),
			output.Truncate(email.Subject, 48),
			flags,
		})
	}
	p.Table([]string{"ID", "AGE", "FROM", "SUBJECT", ""}, rows)
}

func newListCommand() *cobra.Command {
	var (
		limit  int
		page   int
		status string
		all    bool
	)

	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List emails in the mailbox",
		Long: `List emails, newest first.

Use --status to filter by state or action. Use --all to walk every page; the
client paces itself to your tier's rate limit while doing so.`,
		Example: `  tropmail ls
  tropmail ls --status Open --limit 50
  tropmail ls --all --json | jq -r '.emails[] | [.timestamp, .subject] | @tsv'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			client, err := app.Client()
			if err != nil {
				return err
			}

			opts := tropmail.ListOptions{
				Limit:  limit,
				Page:   page,
				Status: tropmail.ListStatus(status),
			}

			if !all {
				result, err := client.Emails.List(cmd.Context(), opts)
				if err != nil {
					return err
				}
				return app.Printer.Print(result, func(p *printer) {
					emailTable(p, result.Emails)
					p.Printf("%s\n", p.Muted(fmt.Sprintf(
						"page %d · %d shown · %d in mailbox",
						result.Page, len(result.Emails), result.Total)))
				})
			}

			var collected []tropmail.Email
			for email, err := range client.Emails.All(cmd.Context(), opts) {
				if err != nil {
					return err
				}
				collected = append(collected, email)
			}
			return app.Printer.Print(
				map[string]any{"emails": collected, "count": len(collected)},
				func(p *printer) {
					emailTable(p, collected)
					p.Printf("%s\n", p.Muted(fmt.Sprintf("%d emails", len(collected))))
				},
			)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "emails per page (1-100)")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	cmd.Flags().StringVarP(&status, "status", "s", "all",
		"filter: all, Open, Close, Favorite, Delete, Block, Phishing, Scam, Malicious")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "fetch every page")
	return cmd
}

func newSearchCommand() *cobra.Command {
	var (
		limit int
		page  int
		all   bool
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search across the mailbox",
		Long: `Search subjects, senders, and bodies.

The API reports a total of 0 for search, so the count shown is what was
fetched, not what exists.`,
		Example: `  tropmail search invoice
  tropmail search "password reset" --all --json`,
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

			query := args[0]
			opts := tropmail.ListOptions{Limit: limit, Page: page}

			if !all {
				result, err := client.Emails.Search(cmd.Context(), query, opts)
				if err != nil {
					return err
				}
				return app.Printer.Print(result, func(p *printer) {
					emailTable(p, result.Emails)
					p.Printf("%s\n", p.Muted(fmt.Sprintf(
						"page %d · %d hits", result.Page, len(result.Emails))))
				})
			}

			var collected []tropmail.Email
			for email, err := range client.Emails.SearchAll(cmd.Context(), query, opts) {
				if err != nil {
					return err
				}
				collected = append(collected, email)
			}
			return app.Printer.Print(
				map[string]any{"emails": collected, "count": len(collected)},
				func(p *printer) {
					emailTable(p, collected)
					p.Printf("%s\n", p.Muted(fmt.Sprintf("%d hits", len(collected))))
				},
			)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "results per page (1-100)")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "fetch every page")
	return cmd
}
