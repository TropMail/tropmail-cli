package cmd

import (
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	tropmail "github.com/tropmail/tropmail-go"

	"github.com/tropmail/tropmail-cli/internal/tui"
)

// runTUI opens the interactive inbox, which is what bare `tropmail` does.
func runTUI(cmd *cobra.Command) error {
	if flags.json {
		return cmd.Help()
	}
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		// Piped with no subcommand: show help rather than emitting escape codes.
		return cmd.Help()
	}

	app, err := newApp()
	if err != nil {
		return err
	}
	client, err := app.Client()
	if err != nil {
		return err
	}
	mailboxID, err := app.ResolveMailboxID(cmd.Context(), client)
	if err != nil {
		return err
	}

	return tui.Run(cmd.Context(), tui.Options{
		Client:    client,
		MailboxID: mailboxID,
		Status:    tropmail.StatusAll,
		PageSize:  50,
		NoColor:   app.Printer.NoColor,
	})
}
