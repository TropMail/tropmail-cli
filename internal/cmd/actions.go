package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	tropmail "github.com/tropmail/tropmail-go"
)

// action describes one state or status change exposed as its own command.
type action struct {
	use     string
	aliases []string
	short   string
	long    string
	apply   func(context.Context, *tropmail.Client, string, string) (*tropmail.ActionResult, error)
	done    string
}

func newActionCommands() []*cobra.Command {
	actions := []action{
		{
			use:   "open",
			short: "Mark emails as opened",
			apply: func(ctx context.Context, c *tropmail.Client, mailboxID, id string) (*tropmail.ActionResult, error) {
				return c.Emails.SetState(ctx, mailboxID, id, tropmail.StateOpen)
			},
			done: "opened",
		},
		{
			use:   "close",
			short: "Mark emails as closed",
			apply: func(ctx context.Context, c *tropmail.Client, mailboxID, id string) (*tropmail.ActionResult, error) {
				return c.Emails.SetState(ctx, mailboxID, id, tropmail.StateClose)
			},
			done: "closed",
		},
		{
			use:     "fav",
			aliases: []string{"favorite", "star"},
			short:   "Flag emails as favorites",
			apply: func(ctx context.Context, c *tropmail.Client, mailboxID, id string) (*tropmail.ActionResult, error) {
				return c.Emails.Favorite(ctx, mailboxID, id)
			},
			done: "favorited",
		},
		{
			use:   "block",
			short: "Block the sender of an email",
			long: `Block the sender of an email.

Blocking applies to the sender address, not just this message: future mail from
them is rejected at delivery. Undo it with 'tropmail unblock'.`,
			apply: func(ctx context.Context, c *tropmail.Client, mailboxID, id string) (*tropmail.ActionResult, error) {
				return c.Emails.Block(ctx, mailboxID, id)
			},
			done: "blocked",
		},
		{
			use:     "unblock",
			aliases: []string{"clear", "unfav"},
			short:   "Clear the action status, unblocking the sender",
			apply: func(ctx context.Context, c *tropmail.Client, mailboxID, id string) (*tropmail.ActionResult, error) {
				return c.Emails.ClearAction(ctx, mailboxID, id)
			},
			done: "cleared",
		},
		{
			use:     "delete",
			aliases: []string{"rm", "trash"},
			short:   "Soft-delete emails",
			long: `Soft-delete emails.

The message is hidden and will be permanently removed after the retention window;
it stays recoverable until then.`,
			apply: func(ctx context.Context, c *tropmail.Client, mailboxID, id string) (*tropmail.ActionResult, error) {
				return c.Emails.Delete(ctx, mailboxID, id)
			},
			done: "deleted",
		},
	}

	commands := make([]*cobra.Command, 0, len(actions))
	for _, item := range actions {
		commands = append(commands, newActionCommand(item))
	}
	return commands
}

func newActionCommand(spec action) *cobra.Command {
	long := spec.long
	if long == "" {
		long = spec.short + "."
	}

	return &cobra.Command{
		Use:     spec.use + " <email-id>...",
		Aliases: spec.aliases,
		Short:   spec.short,
		Long: long + `

Accepts several ids, applying the change to each in turn. Requests are paced to
your tier's rate limit, so bulk changes will not trip a 429.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			results := make([]*tropmail.ActionResult, 0, len(args))
			for _, id := range args {
				result, err := spec.apply(cmd.Context(), client, mailboxID, id)
				if err != nil {
					return fmt.Errorf("%s %s: %w", spec.use, id, err)
				}
				results = append(results, result)
			}

			return app.Printer.Print(
				map[string]any{"action": spec.use, "results": results, "count": len(results)},
				func(p *printer) {
					for i, id := range args {
						line := fmt.Sprintf("%s %s", id, spec.done)
						if results[i] != nil && results[i].SenderEmail != "" {
							line += " (" + results[i].SenderEmail + ")"
						}
						p.Printf("%s\n", line)
					}
				},
			)
		},
	}
}
