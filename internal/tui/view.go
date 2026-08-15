package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tropmail "github.com/tropmail/tropmail-go"

	"github.com/tropmail/tropmail-cli/internal/output"
)

// listWidth gives the list roughly a third of the window, within sane bounds.
func (m Model) listWidth() int {
	width := m.width / 3
	if width < minListWidth {
		width = minListWidth
	}
	if width > maxListWidth {
		width = maxListWidth
	}
	if width > m.width-20 {
		width = maxInt(20, m.width-20)
	}
	return width
}

// paneGap is the gutter between the list and the preview.
const paneGap = 1

func (m Model) previewWidth() int {
	// Two panes, each with a 1-column border on both sides, plus the gutter.
	return maxInt(20, m.width-m.listWidth()-4-paneGap-1)
}

// chromeHeight is everything that is not the two panes: title, status, help.
func (m Model) chromeHeight() int {
	height := 3
	if m.help.ShowAll {
		height += 5
	}
	return height
}

func (m Model) bodyHeight() int {
	return maxInt(3, m.height-m.chromeHeight()-2)
}

// listHeight is how many emails fit in the list pane. Each one occupies two
// lines: the sender row and the subject beneath it.
func (m Model) listHeight() int {
	return maxInt(1, m.bodyHeight()/rowLines)
}

// View renders the whole screen.
func (m Model) View() string {
	if !m.ready {
		return "\n  " + m.spinner.View() + " connecting…\n"
	}

	panes := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.paneStyle(focusList).
			Width(m.listWidth()).Height(m.bodyHeight()).
			MarginRight(paneGap).Render(m.listView()),
		m.paneStyle(focusPreview).
			Width(m.previewWidth()).Height(m.bodyHeight()).Render(m.viewport.View()),
	)

	sections := []string{m.titleView(), panes, m.statusView()}
	if m.help.ShowAll {
		sections = append(sections, m.help.View(m.keys))
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) paneStyle(area focusArea) lipgloss.Style {
	if m.focus == area {
		return m.theme.paneFocused
	}
	return m.theme.paneBorder
}

func (m Model) titleView() string {
	title := m.theme.title.Render("TropMail")

	address := "connecting…"
	counts := ""
	if m.mailbox != nil {
		address = m.mailbox.Email
		counts = m.theme.mutedText.Render(fmt.Sprintf(
			"  %d open · %d closed · %d favorite",
			m.mailbox.OpenedCount, m.mailbox.ClosedCount, m.mailbox.FavoriteCount))
	}

	scope := ""
	switch {
	case m.query != "":
		scope = m.theme.accentText.Render("  search: " + m.query)
	case m.status != "" && m.status != tropmail.StatusAll:
		scope = m.theme.accentText.Render("  filter: " + string(m.status))
	}

	line := title + " " + address + counts + scope
	return output.Truncate(line, maxInt(10, m.width))
}

func (m Model) listView() string {
	if len(m.emails) == 0 {
		if m.loadingPage {
			return m.spinner.View() + " loading…"
		}
		return m.theme.mutedText.Render("no messages")
	}

	rows := m.listHeight()
	width := m.listWidth() - 2
	start := m.scrollOffset(rows)
	end := minInt(len(m.emails), start+rows)

	// Reserve the last row for the "loading more" hint rather than growing the
	// pane past its height, which would push the status bar off screen.
	trailer := ""
	if m.loadingPage && end == len(m.emails) {
		trailer = m.theme.mutedText.Render(m.spinner.View() + " more…")
		if end-start == rows {
			end--
		}
	}

	lines := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(i, width))
	}
	if trailer != "" {
		lines = append(lines, trailer)
	}
	return strings.Join(lines, "\n")
}

// scrollOffset keeps the cursor visible with a little context around it.
func (m Model) scrollOffset(rows int) int {
	if len(m.emails) <= rows {
		return 0
	}
	offset := m.cursor - rows/2
	if offset < 0 {
		offset = 0
	}
	if offset > len(m.emails)-rows {
		offset = len(m.emails) - rows
	}
	return offset
}

func (m Model) renderRow(index, width int) string {
	email := m.emails[index]

	sender := email.From.Address
	if email.From.Name != "" {
		sender = email.From.Name
	}

	age := output.RelativeTime(email.Timestamp)
	marks := rowMarks(email)

	// Mark glyphs are multibyte, so measure display cells rather than bytes.
	senderWidth := maxInt(8, width-lipgloss.Width(age)-lipgloss.Width(marks)-4)
	line := fmt.Sprintf("%-*s %s %s",
		senderWidth, output.Truncate(sender, senderWidth), age, marks)
	subject := "  " + output.Truncate(email.Subject, maxInt(8, width-2))

	style := m.theme.normalRow
	if email.EmailState == tropmail.StateOpen {
		style = m.theme.unreadRow
	}
	if index == m.cursor {
		return m.theme.selectedRow.Width(width).Render(line) + "\n" +
			m.theme.selectedRow.Width(width).Render(subject)
	}
	return style.Render(line) + "\n" + m.theme.mutedText.Render(subject)
}

// rowMarks compresses an email's flags into a couple of glyphs.
func rowMarks(email tropmail.Email) string {
	marks := ""
	if email.ActionStatus != nil {
		switch *email.ActionStatus {
		case tropmail.ActionFavorite:
			marks += "★"
		case tropmail.ActionBlock:
			marks += "⊘"
		case tropmail.ActionDelete:
			marks += "␡"
		case tropmail.ActionPhishing, tropmail.ActionScam, tropmail.ActionMalicious:
			marks += "!"
		case tropmail.ActionNone:
		}
	}
	if email.AttachmentsCount > 0 {
		marks += "@"
	}
	return marks
}

func (m Model) statusView() string {
	if m.mode != modeBrowse {
		label := "search"
		if m.mode == modeFilter {
			label = "filter"
		}
		return m.theme.searchPrompt.Render(label+" › ") + m.input.View()
	}

	left := m.help.View(m.keys)
	if m.message != "" {
		style := m.theme.successText
		if m.isError {
			style = m.theme.errorText
		}
		left = style.Render(output.Truncate(m.message, maxInt(10, m.width/2)))
	}

	right := m.positionText()
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return output.Truncate(left, maxInt(10, m.width))
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) positionText() string {
	if len(m.emails) == 0 {
		return ""
	}
	position := fmt.Sprintf("%d/%d", m.cursor+1, len(m.emails))
	if m.hasMore {
		position += "+"
	}

	limit := m.client.RateLimit()
	if limit.Limit > 0 {
		position += fmt.Sprintf("  %d/%d req", limit.Remaining, limit.Limit)
	}
	return m.theme.mutedText.Render(position)
}
