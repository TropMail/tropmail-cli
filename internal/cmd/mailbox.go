package cmd

import (
	"github.com/spf13/cobra"
)

func newMailboxCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "mailbox",
		Aliases: []string{"box", "whoami"},
		Short:   "Show the mailbox address and message counts",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			client, err := app.Client()
			if err != nil {
				return err
			}
			mailbox, err := client.Mailbox.Get(cmd.Context())
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
