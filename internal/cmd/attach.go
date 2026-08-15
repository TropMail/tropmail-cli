package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	tropmail "github.com/tropmail/tropmail-go"

	"github.com/tropmail/tropmail-cli/internal/output"
)

func newAttachCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "attach",
		Aliases: []string{"att", "attachment"},
		Short:   "Inspect, scan, and download attachments",
	}
	cmd.AddCommand(
		newAttachListCommand(),
		newAttachInfoCommand(),
		newAttachScanCommand(),
		newAttachDownloadCommand(),
	)
	return cmd
}

func newAttachListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "ls <email-id>",
		Aliases: []string{"list"},
		Short:   "List the attachments on an email",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			client, err := app.Client()
			if err != nil {
				return err
			}

			detail, _, err := app.fetchDetail(
				cmd.Context(), client, args[0], tropmail.ViewText, "")
			if err != nil {
				return err
			}

			return app.Printer.Print(
				map[string]any{
					"email_id":    detail.ID,
					"attachments": detail.Attachments,
					"count":       len(detail.Attachments),
				},
				func(p *printer) { renderAttachments(p, detail) },
			)
		},
	}
}

func newAttachInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "info <attachment-id>",
		Aliases: []string{"get"},
		Short:   "Show metadata for one attachment",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp()
			if err != nil {
				return err
			}
			client, err := app.Client()
			if err != nil {
				return err
			}

			info, err := client.Attachments.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			return app.Printer.Print(info, func(p *printer) {
				p.Printf("%s %s\n", p.Bold("file:"), info.Filename)
				if info.Size != nil {
					p.Printf("%s %s\n", p.Bold("size:"), humanSize(*info.Size))
				}
				if info.MimeType != nil {
					p.Printf("%s %s\n", p.Bold("type:"), *info.MimeType)
				}
				p.Printf("%s %s\n", p.Bold("scan:"), info.ScanStatus)
				p.Printf("%s %s\n", p.Bold("mail:"), info.EmailID)
			})
		},
	}
}

func newAttachScanCommand() *cobra.Command {
	var wholeEmail bool

	cmd := &cobra.Command{
		Use:   "scan <id>",
		Short: "Scan an attachment for malware",
		Long: `Scan an attachment for malware.

Scanning is asynchronous: a fresh request comes back as Processing, and the
result lands on a later read. Already-scanned attachments return the cached
verdict without rescanning.`,
		Example: `  tropmail attach scan 018f...            # one attachment
  tropmail attach scan 018f... --email    # every attachment on an email`,
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

			if wholeEmail {
				scans, err := client.Emails.ScanAttachments(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				app.invalidate(args[0])
				return app.Printer.Print(
					map[string]any{"scans": scans, "count": len(scans)},
					func(p *printer) { renderScans(p, scans) },
				)
			}

			scan, err := client.Attachments.Scan(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return app.Printer.Print(scan, func(p *printer) {
				renderScans(p, []tropmail.Scan{*scan})
			})
		},
	}

	cmd.Flags().BoolVarP(&wholeEmail, "email", "e", false,
		"treat the id as an email id and scan all of its attachments")
	return cmd
}

func renderScans(p *printer, scans []tropmail.Scan) {
	rows := make([][]string, 0, len(scans))
	for _, scan := range scans {
		rows = append(rows, []string{
			scan.AttachmentID,
			output.Truncate(scan.Filename, 40),
			string(scan.ScanStatus),
			output.Truncate(scan.Message, 40),
		})
	}
	p.Table([]string{"ID", "FILE", "SCAN", "NOTE"}, rows)
}

func newAttachDownloadCommand() *cobra.Command {
	var (
		outPath    string
		wholeEmail bool
	)

	cmd := &cobra.Command{
		Use:     "download <id>",
		Aliases: []string{"dl"},
		Short:   "Download attachment bytes to a local folder",
		Long: `Download attachment bytes over the authenticated API.

Single attachment:
  tropmail attach download <attachment-id>
  → writes ./<filename> (or -o path)

All attachments on an email:
  tropmail attach download <email-id> --email
  → writes ./downloads/<email-id-prefix>/<filename>
  Override the folder with -o.`,
		Example: `  tropmail attach download 018f... -o invoice.pdf
  tropmail attach download 018f... --email
  tropmail attach download 018f... --email -o ./my-inbox`,
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

			if wholeEmail {
				items, err := client.Emails.DownloadAttachments(cmd.Context(), args[0])
				if err != nil {
					return err
				}

				directory := outPath
				if directory == "" {
					directory = filepath.Join("downloads", shortID(args[0]))
				}
				if err := os.MkdirAll(directory, 0o755); err != nil {
					return fmt.Errorf("create download folder: %w", err)
				}

				saved := make([]map[string]any, 0, len(items))
				skipped := 0
				for _, item := range items {
					if !item.Available {
						skipped++
						continue
					}
					target := uniquePath(directory, safeName(item.Filename, item.AttachmentID))
					written, err := client.Attachments.DownloadTo(
						cmd.Context(), item.AttachmentID, target)
					if err != nil {
						return fmt.Errorf("download %s: %w", item.AttachmentID, err)
					}
					saved = append(saved, map[string]any{
						"path":  target,
						"bytes": written,
						"file":  item.Filename,
					})
				}
				return app.Printer.Print(
					map[string]any{
						"directory": directory,
						"saved":     saved,
						"count":     len(saved),
						"skipped":   skipped,
					},
					func(p *printer) {
						p.Printf("%s %s\n", p.Bold("folder:"), directory)
						if len(saved) == 0 {
							p.Printf("No attachments downloaded")
							if skipped > 0 {
								p.Printf(" (%d unavailable)", skipped)
							}
							p.Printf(".\n")
							return
						}
						for _, item := range saved {
							p.Printf("  %s (%s)\n", item["path"], humanSize(item["bytes"].(int64)))
						}
						if skipped > 0 {
							p.Printf("%s %d unavailable skipped\n", p.Bold("note:"), skipped)
						}
					},
				)
			}

			info, err := client.Attachments.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			target := outPath
			if target == "" {
				target = safeName(info.Filename, args[0])
			} else if strings.HasSuffix(outPath, string(os.PathSeparator)) || isDir(outPath) {
				if err := os.MkdirAll(outPath, 0o755); err != nil {
					return fmt.Errorf("create download folder: %w", err)
				}
				target = uniquePath(outPath, safeName(info.Filename, args[0]))
			}

			written, err := client.Attachments.DownloadTo(cmd.Context(), args[0], target)
			if err != nil {
				return err
			}
			return app.Printer.Print(
				map[string]any{"path": target, "bytes": written, "filename": info.Filename},
				func(p *printer) {
					p.Printf("%s %s (%s)\n", p.Bold("saved:"), target, humanSize(written))
				},
			)
		},
	}

	cmd.Flags().StringVarP(&outPath, "output", "o", "",
		"output file, or directory when downloading an email / trailing slash")
	cmd.Flags().BoolVarP(&wholeEmail, "email", "e", false,
		"treat the id as an email id and download all of its attachments")
	return cmd
}

// safeName keeps a server-supplied filename from escaping the target directory.
func safeName(filename, fallback string) string {
	name := filepath.Base(filepath.Clean("/" + filename))
	if name == "." || name == "/" || name == "" {
		return fallback
	}
	return name
}

func shortID(id string) string {
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) > 8 {
		return clean[:8]
	}
	if clean == "" {
		return "email"
	}
	return clean
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// uniquePath avoids silently overwriting an existing file.
func uniquePath(dir, filename string) string {
	candidate := filepath.Join(dir, filename)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	for i := 2; i < 1000; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, shortID(filename), ext))
}
