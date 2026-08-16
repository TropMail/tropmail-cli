package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMailboxesCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "mailboxes",
		Aliases: []string{"boxes"},
		Short:   "List inboxes this API key can see",
		Example: `  tropmail mailboxes
  tropmail mailboxes --json | jq '.mailboxes[].id'`,
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
			listed, err := client.Mailboxes.List(cmd.Context())
			if err != nil {
				return err
			}

			return app.Printer.Print(listed, func(p *printer) {
				if len(listed.Mailboxes) == 0 {
					p.Printf("No mailboxes on this key.\n")
					return
				}
				rows := make([][]string, 0, len(listed.Mailboxes))
				for _, box := range listed.Mailboxes {
					rows = append(rows, []string{
						box.ID,
						box.Email,
						fmt.Sprintf("%d", box.OpenedCount),
						fmt.Sprintf("%d", box.ClosedCount),
						fmt.Sprintf("%d", box.FavoriteCount),
					})
				}
				p.Table([]string{"ID", "EMAIL", "OPEN", "CLOSED", "FAV"}, rows)
			})
		},
	}
}

func newMailboxCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "mailbox [id]",
		Aliases: []string{"box", "whoami"},
		Short:   "Show one mailbox address and message counts",
		Example: `  tropmail mailbox
  tropmail mailbox 550e8400-e29b-41d4-a716-446655440000`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			client, err := app.Client()
			if err != nil {
				return err
			}
			mailboxID := ""
			if len(args) == 1 {
				mailboxID = args[0]
			} else {
				mailboxID, err = app.ResolveMailboxID(cmd.Context(), client)
				if err != nil {
					return err
				}
			}
			mailbox, err := client.Mailboxes.Get(cmd.Context(), mailboxID)
			if err != nil {
				return err
			}

			return app.Printer.Print(mailbox, func(p *printer) {
				p.Printf("%s\n", p.Accent(mailbox.Email))
				p.Printf("%s %d   %s %d   %s %d\n",
					p.Bold("open"), mailbox.OpenedCount,
					p.Bold("closed"), mailbox.ClosedCount,
					p.Bold("favorite"), mailbox.FavoriteCount,
				)
			})
		},
	}
}
