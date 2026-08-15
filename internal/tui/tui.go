// Package tui implements the interactive inbox reader.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	tropmail "github.com/tropmail/tropmail-go"

	"github.com/tropmail/tropmail-cli/internal/cache"
	"github.com/tropmail/tropmail-cli/internal/output"
)

// prefetchMargin is how close to the end of the loaded list the cursor gets
// before the next page is requested in the background.
const prefetchMargin = 5

const (
	minListWidth = 34
	maxListWidth = 60
)

// rowLines is how many terminal lines one email occupies in the list.
const rowLines = 2

// Options configures the inbox reader.
type Options struct {
	Client   *tropmail.Client
	Cache    *cache.Cache
	Status   tropmail.ListStatus
	PageSize int
	NoColor  bool
}

type focusArea int

const (
	focusList focusArea = iota
	focusPreview
)

type inputMode int

const (
	modeBrowse inputMode = iota
	modeSearch
	modeFilter
)

// Model is the Bubble Tea model for the inbox.
type Model struct {
	ctx      context.Context
	client   *tropmail.Client
	store    *cache.Cache
	theme    theme
	keys     keyMap
	help     help.Model
	spinner  spinner.Model
	viewport viewport.Model
	input    textinput.Model

	emails      []tropmail.Email
	cursor      int
	page        int
	pageSize    int
	hasMore     bool
	loadingPage bool

	query  string
	status tropmail.ListStatus

	mailbox       *tropmail.Mailbox
	detail        *tropmail.EmailDetail
	detailID      string
	detailCached  bool
	view          tropmail.View
	loadingDetail bool
	showAttach    bool

	focus   focusArea
	mode    inputMode
	message string
	isError bool
	fatal   error

	width   int
	height  int
	ready   bool
	noColor bool
}

// Run starts the inbox reader and blocks until the user quits.
func Run(ctx context.Context, opts Options) error {
	if opts.PageSize <= 0 {
		opts.PageSize = 50
	}
	if opts.Status == "" {
		opts.Status = tropmail.StatusAll
	}

	model := newModel(ctx, opts)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := program.Run()
	if err != nil {
		return err
	}
	if finished, ok := final.(Model); ok && finished.fatal != nil {
		return finished.fatal
	}
	return nil
}

func newModel(ctx context.Context, opts Options) Model {
	loader := spinner.New()
	loader.Spinner = spinner.Dot

	search := textinput.New()
	search.Prompt = ""
	search.CharLimit = 200

	helper := help.New()
	helper.ShowAll = false

	return Model{
		ctx:         ctx,
		client:      opts.Client,
		store:       opts.Cache,
		theme:       newTheme(),
		keys:        newKeyMap(),
		help:        helper,
		spinner:     loader,
		input:       search,
		page:        1,
		pageSize:    opts.PageSize,
		status:      opts.Status,
		view:        tropmail.ViewMarkdown,
		loadingPage: true,
		noColor:     opts.NoColor,
	}
}

// Init kicks off the first page and the mailbox summary in parallel.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		loadMailbox(m.ctx, m.client),
		loadPage(m.ctx, m.client, "", m.status, 1, m.pageSize, true),
	)
}

// Update handles every message. It is intentionally split by message type so
// the keyboard handling stays readable.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg), nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case mailboxLoadedMsg:
		if msg.err == nil {
			m.mailbox = msg.mailbox
		}
		return m, nil

	case pageLoadedMsg:
		return m.pageLoaded(msg)

	case detailLoadedMsg:
		return m.detailLoaded(msg)

	case actionDoneMsg:
		return m.actionDone(msg)

	case tea.KeyMsg:
		if m.mode != modeBrowse {
			return m.updateInput(msg)
		}
		return m.updateBrowse(msg)
	}

	return m, nil
}

func (m Model) resize(msg tea.WindowSizeMsg) Model {
	m.width = msg.Width
	m.height = msg.Height
	m.help.Width = msg.Width

	previewWidth := m.previewWidth()
	previewHeight := m.bodyHeight()
	if !m.ready {
		m.viewport = viewport.New(previewWidth, previewHeight)
		m.ready = true
	} else {
		m.viewport.Width = previewWidth
		m.viewport.Height = previewHeight
	}
	m.refreshPreview()
	return m
}

func (m Model) pageLoaded(msg pageLoadedMsg) (tea.Model, tea.Cmd) {
	m.loadingPage = false

	// A late response from a filter the user has since changed is discarded.
	if msg.query != m.query || msg.status != m.status {
		return m, nil
	}
	if msg.err != nil {
		if len(m.emails) == 0 {
			m.fatal = msg.err
			return m, tea.Quit
		}
		return m.withError(msg.err.Error()), nil
	}

	if msg.replace {
		m.emails = msg.list.Emails
		m.cursor = 0
	} else {
		m.emails = append(m.emails, msg.list.Emails...)
	}
	m.page = msg.page
	m.hasMore = len(msg.list.Emails) >= m.pageSize

	if len(m.emails) == 0 {
		m.detail = nil
		m.detailID = ""
		m.refreshPreview()
		return m, nil
	}
	if msg.replace {
		return m, m.openSelected()
	}
	return m, nil
}

func (m Model) detailLoaded(msg detailLoadedMsg) (tea.Model, tea.Cmd) {
	// Ignore a body for a message the user has already scrolled past.
	if current, ok := m.selected(); !ok || current.ID != msg.id {
		return m, nil
	}
	m.loadingDetail = false

	if msg.err != nil {
		return m.withError(msg.err.Error()), nil
	}
	m.detail = msg.detail
	m.detailID = msg.id
	m.detailCached = msg.cached
	m.refreshPreview()
	return m, nil
}

func (m Model) actionDone(msg actionDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.withError(fmt.Sprintf("%s failed: %v", msg.label, msg.err)), nil
	}

	// Reflect the change locally so the list updates without a full refetch.
	for i := range m.emails {
		if m.emails[i].ID != msg.id {
			continue
		}
		if msg.result != nil {
			if msg.result.EmailState != "" {
				m.emails[i].EmailState = msg.result.EmailState
			}
			m.emails[i].ActionStatus = msg.result.ActionStatus
		}
		break
	}

	m.message = msg.label + " " + shortID(msg.id)
	m.isError = false
	return m, loadMailbox(m.ctx, m.client)
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.mode = modeBrowse
		m.input.Blur()
		m.input.SetValue("")
		return m, nil

	case key.Matches(msg, m.keys.Accept):
		value := strings.TrimSpace(m.input.Value())
		mode := m.mode
		m.mode = modeBrowse
		m.input.Blur()
		m.input.SetValue("")

		if mode == modeSearch {
			m.query = value
		} else {
			if value == "" {
				value = string(tropmail.StatusAll)
			}
			m.status = tropmail.ListStatus(value)
			m.query = ""
		}
		m.emails = nil
		m.cursor = 0
		m.page = 1
		m.loadingPage = true
		m.detail = nil
		m.detailID = ""
		m.refreshPreview()
		return m, loadPage(m.ctx, m.client, m.query, m.status, 1, m.pageSize, true)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.viewport.Height = m.bodyHeight()
		return m, nil

	case key.Matches(msg, m.keys.Tab):
		if m.focus == focusList {
			m.focus = focusPreview
		} else {
			m.focus = focusList
		}
		return m, nil

	case key.Matches(msg, m.keys.Search):
		m.mode = modeSearch
		m.input.Placeholder = "search subjects, senders and bodies"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.keys.Filter):
		m.mode = modeFilter
		m.input.Placeholder = "all, Open, Close, Favorite, Delete, Block, Phishing, Scam, Malicious"
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.keys.Refresh):
		m.emails = nil
		m.cursor = 0
		m.page = 1
		m.loadingPage = true
		m.message = "refreshing"
		return m, tea.Batch(
			loadMailbox(m.ctx, m.client),
			loadPage(m.ctx, m.client, m.query, m.status, 1, m.pageSize, true),
		)

	case key.Matches(msg, m.keys.Attach):
		m.showAttach = !m.showAttach
		m.refreshPreview()
		return m, nil

	case key.Matches(msg, m.keys.View):
		m.view = nextView(m.view)
		m.detail = nil
		m.message = "view: " + string(m.view)
		return m, m.openSelected()

	case key.Matches(msg, m.keys.Copy):
		email, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.message = email.ID
		m.isError = false
		return m, copyToClipboard(email.ID)
	}

	if m.focus == focusPreview {
		return m.updatePreview(msg)
	}
	return m.updateList(msg)
}

func (m Model) updatePreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Back) {
		m.focus = focusList
		return m, nil
	}
	if action, label, ok := m.actionFor(msg); ok {
		return m.runAction(action, label)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	previous := m.cursor

	switch {
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.emails)-1 {
			m.cursor++
		}
	case key.Matches(msg, m.keys.PageUp):
		m.cursor = maxInt(0, m.cursor-m.listHeight())
	case key.Matches(msg, m.keys.PageDown):
		m.cursor = minInt(len(m.emails)-1, m.cursor+m.listHeight())
	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
	case key.Matches(msg, m.keys.Bottom):
		m.cursor = maxInt(0, len(m.emails)-1)
	case key.Matches(msg, m.keys.Open):
		m.focus = focusPreview
		return m, m.openSelected()
	default:
		if action, label, ok := m.actionFor(msg); ok {
			return m.runAction(action, label)
		}
		return m, nil
	}

	if m.cursor < 0 {
		m.cursor = 0
	}

	cmds := []tea.Cmd{}
	if m.cursor != previous {
		m.detail = nil
		m.viewport.GotoTop()
		cmds = append(cmds, m.openSelected())
	}
	if cmd := m.maybePrefetch(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// maybePrefetch loads the next page once the cursor nears the end, so scrolling
// never stalls on a network round trip.
func (m *Model) maybePrefetch() tea.Cmd {
	if m.loadingPage || !m.hasMore {
		return nil
	}
	if m.cursor < len(m.emails)-prefetchMargin {
		return nil
	}
	m.loadingPage = true
	return loadPage(m.ctx, m.client, m.query, m.status, m.page+1, m.pageSize, false)
}

func (m *Model) openSelected() tea.Cmd {
	email, ok := m.selected()
	if !ok {
		return nil
	}
	if m.detail != nil && m.detailID == email.ID {
		return nil
	}
	m.loadingDetail = true
	return loadDetail(m.ctx, m.client, m.store, email, m.view)
}

func (m Model) selected() (tropmail.Email, bool) {
	if m.cursor < 0 || m.cursor >= len(m.emails) {
		return tropmail.Email{}, false
	}
	return m.emails[m.cursor], true
}

type actionFunc func(context.Context, *tropmail.Client, string) (*tropmail.ActionResult, error)

// verbs are the two forms of an action used in the status bar: while it is in
// flight and once it lands.
type verbs struct {
	running string
	done    string
}

// actionFor maps a keypress to the mutation it triggers.
func (m Model) actionFor(msg tea.KeyMsg) (actionFunc, verbs, bool) {
	switch {
	case key.Matches(msg, m.keys.Favorite):
		return func(ctx context.Context, c *tropmail.Client, id string) (*tropmail.ActionResult, error) {
			return c.Emails.Favorite(ctx, id)
		}, verbs{"favoriting…", "favorited"}, true

	case key.Matches(msg, m.keys.Block):
		return func(ctx context.Context, c *tropmail.Client, id string) (*tropmail.ActionResult, error) {
			return c.Emails.Block(ctx, id)
		}, verbs{"blocking…", "blocked"}, true

	case key.Matches(msg, m.keys.Delete):
		return func(ctx context.Context, c *tropmail.Client, id string) (*tropmail.ActionResult, error) {
			return c.Emails.Delete(ctx, id)
		}, verbs{"deleting…", "deleted"}, true

	case key.Matches(msg, m.keys.MarkOpen):
		return func(ctx context.Context, c *tropmail.Client, id string) (*tropmail.ActionResult, error) {
			return c.Emails.SetState(ctx, id, tropmail.StateOpen)
		}, verbs{"opening…", "opened"}, true

	case key.Matches(msg, m.keys.MarkClose):
		return func(ctx context.Context, c *tropmail.Client, id string) (*tropmail.ActionResult, error) {
			return c.Emails.SetState(ctx, id, tropmail.StateClose)
		}, verbs{"closing…", "closed"}, true

	case key.Matches(msg, m.keys.Clear):
		return func(ctx context.Context, c *tropmail.Client, id string) (*tropmail.ActionResult, error) {
			return c.Emails.ClearAction(ctx, id)
		}, verbs{"clearing…", "cleared"}, true
	}
	return nil, verbs{}, false
}

func (m Model) runAction(run actionFunc, label verbs) (tea.Model, tea.Cmd) {
	email, ok := m.selected()
	if !ok {
		return m, nil
	}
	m.message = label.running
	m.isError = false
	return m, applyAction(m.ctx, m.client, m.store, email, label.done, run)
}

func (m Model) withError(text string) Model {
	m.message = text
	m.isError = true
	return m
}

func nextView(current tropmail.View) tropmail.View {
	switch current {
	case tropmail.ViewMarkdown:
		return tropmail.ViewText
	case tropmail.ViewText:
		return tropmail.ViewHTML
	default:
		return tropmail.ViewMarkdown
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// renderMarkdown styles a markdown body for the preview pane.
func (m Model) renderMarkdown(body string, width int) string {
	options := []glamour.TermRendererOption{glamour.WithWordWrap(maxInt(20, width-2))}
	if m.noColor {
		options = append(options, glamour.WithStandardStyle("notty"))
	} else {
		options = append(options, glamour.WithAutoStyle())
	}

	renderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return body
	}
	rendered, err := renderer.Render(body)
	if err != nil {
		return body
	}
	return rendered
}

func (m Model) previewLines() string {
	if m.detail == nil {
		if m.loadingDetail {
			return m.spinner.View() + " loading…"
		}
		if len(m.emails) == 0 {
			return m.theme.mutedText.Render("no messages")
		}
		return m.theme.mutedText.Render("press enter to read")
	}

	width := m.previewWidth()
	var out strings.Builder

	label := m.theme.headerLabel
	out.WriteString(label.Render("From    ") + " " + addressText(m.detail.From) + "\n")
	if len(m.detail.To) > 0 {
		out.WriteString(label.Render("To      ") + " " +
			output.Truncate(addressListText(m.detail.To), width-10) + "\n")
	}
	out.WriteString(label.Render("Subject ") + " " + m.detail.Subject + "\n")
	out.WriteString(label.Render("Date    ") + " " + m.detail.Timestamp + "\n")

	meta := string(m.detail.EmailState)
	if m.detail.ActionStatus != nil && *m.detail.ActionStatus != "" {
		meta += " · " + string(*m.detail.ActionStatus)
	}
	meta += " · " + string(m.view)
	if m.detailCached {
		meta += " · cached"
	}
	out.WriteString(m.theme.mutedText.Render(meta) + "\n")
	out.WriteString(m.theme.mutedText.Render(strings.Repeat("─", maxInt(4, width))) + "\n\n")

	if m.showAttach {
		out.WriteString(m.attachmentBlock(width))
		return out.String()
	}

	body := m.detail.Content
	if m.view == tropmail.ViewMarkdown {
		body = m.renderMarkdown(body, width)
	} else {
		body = lipgloss.NewStyle().Width(maxInt(20, width)).Render(body)
	}
	out.WriteString(strings.TrimRight(body, "\n"))
	return out.String()
}

func (m Model) attachmentBlock(width int) string {
	if len(m.detail.Attachments) == 0 {
		return m.theme.mutedText.Render("no attachments")
	}

	var out strings.Builder
	out.WriteString(m.theme.headerLabel.Render("Attachments") + "\n\n")
	for _, attachment := range m.detail.Attachments {
		name := "(unnamed)"
		if attachment.Filename != nil && *attachment.Filename != "" {
			name = *attachment.Filename
		}
		size := ""
		if attachment.Size != nil {
			size = " · " + humanSize(*attachment.Size)
		}
		out.WriteString(output.Truncate(name, width-4) + "\n")
		out.WriteString(m.theme.mutedText.Render(
			"  "+attachment.AttachmentID+size+" · "+string(attachment.ScanStatus)) + "\n\n")
	}
	out.WriteString(m.theme.mutedText.Render("press a to return to the body"))
	return out.String()
}

func (m *Model) refreshPreview() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.previewLines())
}

func addressText(address tropmail.Address) string {
	if address.Name != "" {
		return fmt.Sprintf("%s <%s>", address.Name, address.Address)
	}
	return address.Address
}

func addressListText(addresses []tropmail.Address) string {
	parts := make([]string, 0, len(addresses))
	for _, address := range addresses {
		parts = append(parts, addressText(address))
	}
	return strings.Join(parts, ", ")
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGT"[exp])
}
