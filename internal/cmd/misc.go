package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/tropmail/tropmail-cli/internal/cache"
	"github.com/tropmail/tropmail-cli/internal/output"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			info := map[string]string{
				"version": Version,
				"commit":  Commit,
				"date":    Date,
				"go":      runtime.Version(),
				"os":      runtime.GOOS,
				"arch":    runtime.GOARCH,
			}
			// Deliberately avoids newApp: `version` must work with no config.
			out := output.New(flags.json, flags.quiet, flags.noColor)
			return out.Print(info, func(p *printer) {
				p.Printf("tropmail %s (%s, %s) %s/%s\n",
					Version, Commit, Date, runtime.GOOS, runtime.GOARCH)
			})
		},
	}
}

func newCacheCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect and clear the local body cache",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the cache directory",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			dir, err := cache.Dir()
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, dir)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "clear",
		Short: "Delete every cached email body",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			removed, err := cache.New(true).Clear()
			if err != nil {
				return err
			}
			return app.Printer.Print(
				map[string]any{"removed": removed},
				func(p *printer) { p.Printf("Removed %d cached bodies\n", removed) },
			)
		},
	})

	return cmd
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "completion <bash|zsh|fish|powershell>",
		Short: "Generate a shell completion script",
		Long: `Generate a shell completion script.

  bash:  source <(tropmail completion bash)
  zsh:   tropmail completion zsh > "${fpath[1]}/_tropmail"
  fish:  tropmail completion fish > ~/.config/fish/completions/tropmail.fish`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(_ *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return root.GenZshCompletion(os.Stdout)
			case "fish":
				return root.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
}
