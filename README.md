# tropmail

The command line client for the [TropMail API](https://api.tropmail.com):
an interactive inbox reader and a set of scriptable subcommands that speak JSON.

```bash
brew install tropmail/tap/tropmail
go install github.com/tropmail/tropmail-cli@latest
```

Binaries for linux, macOS, and Windows on amd64 and arm64 are attached to each
[release](https://github.com/tropmail/tropmail-cli/releases). There is also a
container image:

```bash
docker run --rm -e TROPMAIL_API_KEY ghcr.io/tropmail/tropmail ls --json
```

## Getting started

```bash
tropmail auth login          # prompts, validates, stores in the OS keyring
tropmail                     # opens the inbox
```

`auth login` checks the key against the API before saving it, so a typo fails
immediately rather than on your next command.

## The inbox

Running `tropmail` with no arguments opens a two-pane reader: the message list
on the left, the rendered body on the right.

| Key | Action |
|---|---|
| `j` / `k`, `↓` / `↑` | move |
| `ctrl+d` / `ctrl+u` | page |
| `g` / `G` | top / bottom |
| `enter` | read the selected message |
| `tab` | switch panes |
| `esc` | back to the list |
| `/` | search |
| `s` | filter by status |
| `r` | refresh |
| `v` | cycle markdown, text, HTML |
| `a` | attachments |
| `y` | copy the message id |
| `o` / `c` | mark opened / closed |
| `f` | favorite |
| `b` | block the sender |
| `u` | clear the action, unblocking the sender |
| `d` | delete |
| `?` | full help |
| `q` | quit |

Markdown is rendered with [Glamour](https://github.com/charmbracelet/glamour).
The next page loads in the background as you approach the end of the list, so
scrolling does not stall on the network.

## Scripting

Every subcommand takes `--json`, which makes the CLI a usable API client for
shell scripts and AI agents.

```bash
# The subject of every unread message
tropmail ls --status Open --json | jq -r '.emails[].subject'

# Read the newest message as plain markdown
tropmail read "$(tropmail ls --limit 1 --json | jq -r '.emails[0].id')" --raw

# Favorite everything from one sender
tropmail search "billing@stripe.com" --all --json \
  | jq -r '.emails[].id' \
  | xargs tropmail fav

# Save every attachment on a message (default: ./downloads/<id-prefix>/)
tropmail attach download <email-id> --email
tropmail attach download <email-id> --email -o ./my-inbox

# Run a hook whenever mail arrives
tropmail watch --exec 'notify-send "TropMail" "$TROPMAIL_SUBJECT"'
```

`watch` prints one NDJSON object per new message under `--json`, and exposes
`$TROPMAIL_ID`, `$TROPMAIL_SUBJECT`, `$TROPMAIL_FROM`, and `$TROPMAIL_TIMESTAMP`
to the `--exec` hook, so subjects containing shell syntax cannot break quoting.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | error |
| 2 | usage |
| 3 | authentication or tier |
| 4 | not found |
| 5 | rate limited |

Under `--json`, errors are written to stderr as JSON with the status, message,
and request id.

## Commands

| Command | Purpose |
|---|---|
| `auth login\|status\|logout\|profiles` | manage API keys and profiles |
| `mailbox` | address and message counts |
| `ls` | list messages, `--status`, `--all` |
| `search <query>` | full-text search |
| `read <id>` | print one message, `--view`, `--raw` |
| `open\|close\|fav\|block\|unblock\|delete <id>...` | change state or status |
| `attach ls\|info\|scan\|download` | attachments |
| `watch` | poll for new mail |
| `cache path\|clear` | manage the body cache |
| `completion <shell>` | shell completions |
| `version` | version, commit, build date |

Global flags: `--json`, `--api-key`, `--base-url`, `--profile`, `--timeout`,
`--no-color`, `--quiet`, `--no-cache`, `--throttle`.

## Configuration

Profiles live in `~/.config/tropmail/config.toml` (or `$TROPMAIL_CONFIG_DIR`):

```toml
default_profile = "default"

[profiles.default]
email = "quiet-otter-1423@tropmail.com"
tier = "Pro"
# base_url = "https://api.tropmail.com/api/v1"  # default; set only to override
```

API keys never go in that file. They go to the OS keyring, falling back to a
`0600` credentials file on headless machines with no secret service.
`TROPMAIL_API_KEY` overrides whatever is stored, which is what CI should use.

### Speed

The binary is static, loads config lazily, and makes no network call on
startup. Message bodies are immutable once delivered, so they are cached under
`~/.cache/tropmail` (or `$TROPMAIL_CACHE_DIR`) and re-reads cost nothing. This
matters most for the markdown view, which the server can take up to a minute to
produce. Actions invalidate the cached copies of the message they touch; pass
`--no-cache` to bypass the cache entirely.

Requests are paced against your tier's rate limit (Pro 3/s, Ultimate 10/s,
Enterprise 50/s) with a token bucket, so bulk operations do not collect `429`s.
Pass `--throttle=false` if you would rather manage that yourself.

## Development

```bash
make build   # ./tropmail
make test
make lint
```

The CLI is built on [tropmail-go](https://github.com/tropmail/tropmail-go).
Pin a released version of that module in `go.mod`.

## Docs

Guides and API reference: [docs.tropmail.com](https://docs.tropmail.com/sdks/cli/).

## License

MIT
