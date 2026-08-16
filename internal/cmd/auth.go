package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	tropmail "github.com/tropmail/tropmail-go"
	"golang.org/x/term"

	"github.com/tropmail/tropmail-cli/internal/config"
)

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage API keys and profiles",
	}
	cmd.AddCommand(newAuthLoginCommand(), newAuthStatusCommand(), newAuthLogoutCommand(),
		newAuthProfilesCommand())
	return cmd
}

func newAuthLoginCommand() *cobra.Command {
	var keyFlag string
	var stdinFlag bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store an API key for a profile",
		Long: `Store an API key in the OS keyring, or a restricted credentials file when no keyring exists.

The key is validated against the API before being saved, so a typo fails here
rather than on your next command.`,
		Example: `  tropmail auth login
  tropmail auth login --key $TROPMAIL_API_KEY
  echo "$KEY" | tropmail auth login --stdin --profile work`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}

			apiKey, err := readAPIKey(keyFlag, stdinFlag)
			if err != nil {
				return err
			}
			if err := tropmail.ValidateAPIKey(apiKey); err != nil {
				return err
			}

			baseURL := flags.baseURL
			if baseURL == "" {
				baseURL = app.Config.Get(app.Profile).Endpoint()
			}
			client, err := tropmail.New(apiKey,
				tropmail.WithBaseURL(baseURL),
				tropmail.WithTimeout(flags.timeout),
				tropmail.WithUserAgent("tropmail-cli/"+Version),
			)
			if err != nil {
				return err
			}

			listed, err := client.Mailboxes.List(cmd.Context())
			if err != nil {
				return fmt.Errorf("key rejected by %s: %w", baseURL, err)
			}
			boxes := listed.Mailboxes
			if len(boxes) == 0 {
				return fmt.Errorf("key accepted by %s but has no mailboxes", baseURL)
			}

			storage, err := config.StoreKey(app.Profile, apiKey)
			if err != nil {
				return err
			}

			profile := app.Config.Get(app.Profile)
			profile.Email = boxes[0].Email
			if len(boxes) == 1 {
				profile.MailboxID = boxes[0].ID
			} else {
				profile.MailboxID = ""
			}
			if flags.baseURL != "" {
				profile.BaseURL = flags.baseURL
			}
			app.Config.Set(app.Profile, profile)
			if err := app.Config.Save(); err != nil {
				return err
			}

			return app.Printer.Print(map[string]any{
				"profile":   app.Profile,
				"email":     profile.Email,
				"mailboxes": listed.Mailboxes,
				"storage":   string(storage),
				"base_url":  baseURL,
			}, func(p *printer) {
				p.Printf("Logged in (%d mailbox", len(boxes))
				if len(boxes) != 1 {
					p.Printf("es")
				}
				p.Printf(")\n")
				for _, box := range boxes {
					p.Printf("  %s  %s\n", p.Accent(box.Email), p.Muted(box.ID))
				}
				p.Printf("%s\n", p.Muted(fmt.Sprintf(
					"profile %q, key stored in the %s", app.Profile, storage)))
				if len(boxes) > 1 {
					p.Printf("Pass --mailbox <id> (or TROPMAIL_MAILBOX_ID) on later commands.\n")
				}
				if storage == config.StorageFile {
					p.Warnf("No OS keyring available; the key is in a restricted credentials file.\n")
				}
			})
		},
	}

	cmd.Flags().StringVar(&keyFlag, "key", "", "API key (skips the prompt)")
	cmd.Flags().BoolVar(&stdinFlag, "stdin", false, "read the API key from stdin")
	return cmd
}

// readAPIKey resolves a key from a flag, stdin, or an interactive prompt.
func readAPIKey(keyFlag string, fromStdin bool) (string, error) {
	if keyFlag != "" {
		return strings.TrimSpace(keyFlag), nil
	}
	if fromStdin {
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return "", errors.New("no API key on stdin")
		}
		return strings.TrimSpace(scanner.Text()), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("not a terminal: pass --key or --stdin")
	}

	fmt.Fprint(os.Stderr, "API key: ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read API key: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func newAuthStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active profile and verify its key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}

			_, storage, err := config.LoadKey(app.Profile)
			if err != nil && flags.apiKey == "" {
				return fmt.Errorf("profile %q is not logged in: %w", app.Profile, err)
			}
			if flags.apiKey != "" {
				storage = "flag"
			}

			client, err := app.Client()
			if err != nil {
				return err
			}
			listed, err := client.Mailboxes.List(cmd.Context())
			if err != nil {
				return err
			}

			limit := client.RateLimit()
			remembered := app.Config.Get(app.Profile).MailboxID
			return app.Printer.Print(map[string]any{
				"profile":    app.Profile,
				"mailboxes":  listed.Mailboxes,
				"mailbox_id": remembered,
				"storage":    string(storage),
				"base_url":   client.BaseURL(),
				"rate_limit": map[string]any{
					"limit":     limit.Limit,
					"remaining": limit.Remaining,
				},
			}, func(p *printer) {
				p.Printf("%s  %s\n", p.Bold("profile"), app.Profile)
				if remembered != "" {
					p.Printf("%s %s\n", p.Bold("mailbox"), remembered)
				}
				for _, box := range listed.Mailboxes {
					p.Printf("%s    %s  %s\n", p.Bold("inbox"), box.Email, p.Muted(box.ID))
				}
				p.Printf("%s  %s\n", p.Bold("api key"), string(storage))
				p.Printf("%s %s\n", p.Bold("endpoint"), client.BaseURL())
				if limit.Limit > 0 {
					p.Printf("%s   %d/%d requests remaining this second\n",
						p.Bold("limit"), limit.Remaining, limit.Limit)
				}
			})
		},
	}
}

func newAuthLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the stored API key for a profile",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			if err := config.DeleteKey(app.Profile); err != nil {
				if errors.Is(err, config.ErrNoCredentials) {
					return fmt.Errorf("profile %q is not logged in", app.Profile)
				}
				return err
			}
			return app.Printer.Print(
				map[string]any{"profile": app.Profile, "logged_out": true},
				func(p *printer) { p.Printf("Removed the API key for profile %q\n", app.Profile) },
			)
		},
	}
}

func newAuthProfilesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List configured profiles",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}

			names := app.Config.Names()
			type row struct {
				Name    string `json:"name"`
				Email   string `json:"email,omitempty"`
				Tier    string `json:"tier,omitempty"`
				Default bool   `json:"default"`
			}
			rows := make([]row, 0, len(names))
			for _, name := range names {
				profile := app.Config.Get(name)
				rows = append(rows, row{
					Name:    name,
					Email:   profile.Email,
					Tier:    profile.Tier,
					Default: name == app.Config.DefaultProfile,
				})
			}

			return app.Printer.Print(
				map[string]any{"profiles": rows, "active": app.Profile},
				func(p *printer) {
					table := make([][]string, 0, len(rows))
					for _, r := range rows {
						marker := " "
						if r.Default {
							marker = "*"
						}
						table = append(table, []string{marker, r.Name, r.Email, r.Tier})
					}
					p.Table([]string{"", "PROFILE", "EMAIL", "TIER"}, table)
				},
			)
		},
	}
}
