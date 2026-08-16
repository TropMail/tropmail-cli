// Package cmd wires the tropmail CLI commands together.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	tropmail "github.com/tropmail/tropmail-go"

	"github.com/tropmail/tropmail-cli/internal/config"
	"github.com/tropmail/tropmail-cli/internal/output"
)

// Exit codes, so scripts can branch without parsing messages.
const (
	ExitOK        = 0
	ExitError     = 1
	ExitUsage     = 2
	ExitAuth      = 3
	ExitNotFound  = 4
	ExitRateLimit = 5
)

// Version is overridden at build time via -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

type globalFlags struct {
	json     bool
	apiKey   string
	baseURL  string
	profile  string
	mailbox  string
	timeout  time.Duration
	noColor  bool
	quiet    bool
	throttle bool
}

var flags = globalFlags{throttle: true}

// printer is a local alias that keeps the human-rendering closures readable.
type printer = output.Printer

// App carries everything a command needs: rendering, credentials, and a client.
type App struct {
	Printer *output.Printer
	Config  *config.Config
	Profile string
}

// newApp assembles the shared context without touching the network.
func newApp() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return &App{
		Printer: output.New(flags.json, flags.quiet, flags.noColor),
		Config:  cfg,
		Profile: cfg.ResolveName(flags.profile),
	}, nil
}

// Client builds an API client for the active profile.
//
// Credentials are resolved lazily so commands such as `version` and
// `completion` never touch the keyring.
func (a *App) Client() (*tropmail.Client, error) {
	apiKey := flags.apiKey
	if apiKey == "" {
		stored, _, err := config.LoadKey(a.Profile)
		if err != nil {
			if errors.Is(err, config.ErrNoCredentials) {
				return nil, fmt.Errorf(
					"no API key for profile %q: run 'tropmail auth login' or set TROPMAIL_API_KEY",
					a.Profile,
				)
			}
			return nil, err
		}
		apiKey = stored
	}

	baseURL := flags.baseURL
	if baseURL == "" {
		baseURL = a.Config.Get(a.Profile).Endpoint()
	}

	return tropmail.New(apiKey,
		tropmail.WithBaseURL(baseURL),
		tropmail.WithTimeout(flags.timeout),
		tropmail.WithThrottle(flags.throttle),
		tropmail.WithUserAgent("tropmail-cli/"+Version),
	)
}

// ResolveMailboxID picks the inbox for mail commands.
//
// Precedence: --mailbox, TROPMAIL_MAILBOX_ID, the profile's remembered id, then
// the inbox list when that list contains exactly one inbox. Multiple inboxes
// fail with a copy-paste example rather than guessing.
func (a *App) ResolveMailboxID(ctx context.Context, client *tropmail.Client) (string, error) {
	if flags.mailbox != "" {
		return flags.mailbox, nil
	}
	if env := os.Getenv("TROPMAIL_MAILBOX_ID"); env != "" {
		return env, nil
	}
	if remembered := a.Config.Get(a.Profile).MailboxID; remembered != "" {
		return remembered, nil
	}

	listed, err := client.Mailboxes.List(ctx)
	if err != nil {
		return "", err
	}
	boxes := listed.Mailboxes
	switch len(boxes) {
	case 0:
		return "", errors.New("this API key has no mailboxes")
	case 1:
		return boxes[0].ID, nil
	default:
		return "", fmt.Errorf(
			"more than one mailbox; pass --mailbox <id>\n  tropmail ls --mailbox %s\n  tropmail mailboxes",
			boxes[0].ID,
		)
	}
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		printError(err)
		return exitCodeFor(err)
	}
	return ExitOK
}

// NewRootCommand exposes the command tree for tests.
func NewRootCommand() *cobra.Command { return newRootCommand() }

func printError(err error) {
	if flags.json {
		printer := output.New(true, false, true)
		printer.Out = output.Stderr
		var apiErr *tropmail.Error
		payload := map[string]any{"error": err.Error()}
		if errors.As(err, &apiErr) {
			payload["status"] = apiErr.Status
			payload["message"] = apiErr.Message
			if apiErr.RequestID != "" {
				payload["request_id"] = apiErr.RequestID
			}
		}
		_ = printer.PrintJSON(payload)
		return
	}
	fmt.Fprintln(output.Stderr, "Error: "+err.Error())
}

func exitCodeFor(err error) int {
	switch {
	case tropmail.IsAuth(err), tropmail.IsTier(err):
		return ExitAuth
	case tropmail.IsNotFound(err):
		return ExitNotFound
	case tropmail.IsRateLimit(err):
		return ExitRateLimit
	default:
		return ExitError
	}
}

func newRootCommand() *cobra.Command {
	// Reset so a second command tree (in tests) does not inherit stale flags.
	flags = globalFlags{throttle: true}

	root := &cobra.Command{
		Use:   "tropmail",
		Short: "Read your TropMail inbox from the terminal",
		Long: `tropmail is the command line client for the TropMail API.

Run it with no arguments to open the interactive inbox. Every subcommand also
works non-interactively and supports --json, so it composes with jq, scripts,
and AI agents.`,
		Example: `  # Open the interactive inbox
  tropmail

  # List unread mail as JSON
  tropmail ls --status Open --json | jq '.emails[].subject'

  # Read the newest message as markdown
  tropmail read $(tropmail ls --limit 1 --json | jq -r '.emails[0].id')`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTUI(cmd)
		},
	}

	persistent := root.PersistentFlags()
	persistent.BoolVar(&flags.json, "json", false, "output machine-readable JSON")
	persistent.StringVar(&flags.apiKey, "api-key", "", "API key (overrides the stored credential)")
	persistent.StringVar(&flags.baseURL, "base-url", "", "API base URL")
	persistent.StringVar(&flags.mailbox, "mailbox", "", "mailbox UUID (or TROPMAIL_MAILBOX_ID)")
	persistent.StringVarP(&flags.profile, "profile", "p", "", "configuration profile to use")
	persistent.DurationVar(&flags.timeout, "timeout", 120*time.Second, "per-request timeout")
	persistent.BoolVar(&flags.noColor, "no-color", false, "disable coloured output")
	persistent.BoolVarP(&flags.quiet, "quiet", "q", false, "suppress non-essential output")
	persistent.BoolVar(&flags.throttle, "throttle", true, "pace requests to the plan rate limit")

	root.AddCommand(
		newAuthCommand(),
		newMailboxesCommand(),
		newMailboxCommand(),
		newListCommand(),
		newSearchCommand(),
		newReadCommand(),
		newAttachCommand(),
		newWatchCommand(),
		newCompletionCommand(root),
		newVersionCommand(),
	)
	root.AddCommand(newActionCommands()...)
	return root
}
