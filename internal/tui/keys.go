package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds every binding, with vim motions alongside the arrow keys.
type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
	Top       key.Binding
	Bottom    key.Binding
	Open      key.Binding
	Back      key.Binding
	Tab       key.Binding
	Search    key.Binding
	Filter    key.Binding
	Refresh   key.Binding
	Favorite  key.Binding
	Block     key.Binding
	Delete    key.Binding
	MarkOpen  key.Binding
	MarkClose key.Binding
	Clear     key.Binding
	Attach    key.Binding
	View      key.Binding
	Copy      key.Binding
	Help      key.Binding
	Quit      key.Binding
	Accept    key.Binding
	Cancel    key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down: key.NewBinding(
			key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u", "page up")),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d", "page down")),
		Top: key.NewBinding(
			key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		Open: key.NewBinding(
			key.WithKeys("enter", "l", "right"), key.WithHelp("enter", "read")),
		Back: key.NewBinding(
			key.WithKeys("esc", "h", "left"), key.WithHelp("esc", "back")),
		Tab: key.NewBinding(
			key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
		Search: key.NewBinding(
			key.WithKeys("/"), key.WithHelp("/", "search")),
		Filter: key.NewBinding(
			key.WithKeys("s"), key.WithHelp("s", "filter")),
		Refresh: key.NewBinding(
			key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Favorite: key.NewBinding(
			key.WithKeys("f"), key.WithHelp("f", "favorite")),
		Block: key.NewBinding(
			key.WithKeys("b"), key.WithHelp("b", "block sender")),
		Delete: key.NewBinding(
			key.WithKeys("d"), key.WithHelp("d", "delete")),
		MarkOpen: key.NewBinding(
			key.WithKeys("o"), key.WithHelp("o", "mark open")),
		MarkClose: key.NewBinding(
			key.WithKeys("c"), key.WithHelp("c", "mark closed")),
		Clear: key.NewBinding(
			key.WithKeys("u"), key.WithHelp("u", "clear action")),
		Attach: key.NewBinding(
			key.WithKeys("a"), key.WithHelp("a", "attachments")),
		View: key.NewBinding(
			key.WithKeys("v"), key.WithHelp("v", "cycle view")),
		Copy: key.NewBinding(
			key.WithKeys("y"), key.WithHelp("y", "yank id")),
		Help: key.NewBinding(
			key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Accept: key.NewBinding(
			key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Cancel: key.NewBinding(
			key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

// ShortHelp is the one-line hint shown in the status bar.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.Search, k.Favorite, k.Help, k.Quit}
}

// FullHelp is the grid shown when help is expanded.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom},
		{k.Open, k.Back, k.Tab, k.View, k.Attach, k.Copy},
		{k.Search, k.Filter, k.Refresh},
		{k.MarkOpen, k.MarkClose, k.Favorite, k.Block, k.Delete, k.Clear},
		{k.Help, k.Quit},
	}
}
