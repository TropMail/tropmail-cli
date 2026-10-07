package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	tropmail "github.com/tropmail/tropmail-go"

	"github.com/tropmail/tropmail-cli/internal/output"
)

// minInterval keeps polling well inside even the smallest tier budget.
const minInterval = 2 * time.Second

func newWatchCommand() *cobra.Command {
	var (
		interval time.Duration
		status   string
		execArgs string
		once     bool
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Print new mail as it arrives",
		Long: `Poll the mailbox and print each new email once.

Polling is paced against your tier's rate limit, and only ids not seen earlier
in the session are printed, so this is safe to leave running. With --json each
email is emitted as one line of NDJSON, which pipes cleanly into jq or a script.`,
		Example: `  tropmail watch
  tropmail watch --interval 30s --status Open
  tropmail watch --json | jq -r '.subject'
  tropmail watch --exec 'notify-send "New mail"'`,
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
			mailboxID, err := app.ResolveMailboxID(cmd.Context(), client)
			if err != nil {
				return err
			}

			if interval < minInterval {
				interval = minInterval
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			opts := tropmail.ListOptions{MailboxID: mailboxID, Limit: 25, Status: tropmail.ListStatus(status)}
			seen := map[string]bool{}

			// The first poll establishes a baseline so a full mailbox does not
			// scroll past on startup.
			first, err := client.Emails.List(ctx, opts)
			if err != nil {
				return err
			}
			for _, email := range first.Emails {
				seen[email.ID] = true
			}
			if !app.Printer.JSON {
				app.Printer.Printf("%s\n", app.Printer.Muted(fmt.Sprintf(
					"watching %s every %s — ctrl-c to stop",
					client.BaseURL(), interval)))
			}
			if once {
				return nil
			}

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
				}

				page, err := client.Emails.List(ctx, opts)
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					// A transient failure should not end a long-lived watch.
					app.Printer.Warnf("poll failed: %v\n", err)
					continue
				}

				for i := len(page.Emails) - 1; i >= 0; i-- {
					email := page.Emails[i]
					if seen[email.ID] {
						continue
					}
					seen[email.ID] = true
					emitArrival(app, email)
					runHook(ctx, app, execArgs, email)
				}
			}
		},
	}

	cmd.Flags().DurationVarP(&interval, "interval", "i", 15*time.Second,
		"poll interval (minimum 2s)")
	cmd.Flags().StringVarP(&status, "status", "s", "all", "filter: all, Open, Close, Favorite, Block, Phishing, Scam, Malicious")
	cmd.Flags().StringVar(&execArgs, "exec", "",
		"shell command to run per email; $TROPMAIL_ID, $TROPMAIL_SUBJECT and $TROPMAIL_FROM are set")
	cmd.Flags().BoolVar(&once, "once", false, "establish the baseline and exit (useful in tests)")
	return cmd
}

func emitArrival(app *App, email tropmail.Email) {
	if app.Printer.JSON {
		_ = app.Printer.PrintJSON(email)
		return
	}
	sender := email.From.Address
	if email.From.Name != "" {
		sender = email.From.Name
	}
	app.Printer.Printf("%s  %s  %s\n",
		app.Printer.Muted(output.RelativeTime(email.Timestamp)),
		app.Printer.Accent(output.Truncate(sender, 24)),
		output.Truncate(email.Subject, 60),
	)
}

// runHook executes the --exec command with the email exposed as environment
// variables, which avoids quoting bugs in subjects that contain shell syntax.
func runHook(ctx context.Context, app *App, command string, email tropmail.Email) {
	if strings.TrimSpace(command) == "" {
		return
	}
	hook := exec.CommandContext(ctx, "sh", "-c", command)
	hook.Env = append(os.Environ(),
		"TROPMAIL_ID="+email.ID,
		"TROPMAIL_SUBJECT="+email.Subject,
		"TROPMAIL_FROM="+email.From.Address,
		"TROPMAIL_TIMESTAMP="+email.Timestamp,
	)
	hook.Stdout = app.Printer.Out
	hook.Stderr = app.Printer.Err
	if err := hook.Run(); err != nil && ctx.Err() == nil {
		app.Printer.Warnf("exec hook failed: %v\n", err)
	}
}
