package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap defines a set of keybindings.
type keyMap struct {
	Up           key.Binding
	Down         key.Binding
	Top          key.Binding
	Bottom       key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	Left         key.Binding
	Right        key.Binding
	Home         key.Binding
	Select       key.Binding
	Download     key.Binding
	Copy         key.Binding
	Open         key.Binding
	Edit         key.Binding
	Refresh      key.Binding
	Search       key.Binding
	Esc          key.Binding
	Help         key.Binding
	Messages     key.Binding
	Info         key.Binding
	Quit         key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.Select, k.Download, k.Copy, k.Info, k.Help, k.Quit}
}

// OrderedHelp returns all keybindings ordered from most used to least used.
func (k keyMap) OrderedHelp() []key.Binding {
	return []key.Binding{
		k.Up,
		k.Down,
		k.Left,
		k.Right,
		k.Select,
		k.Download,
		k.Copy,
		k.Open,
		k.Edit,
		k.Search,
		k.Refresh,
		k.Esc,
		k.Top,
		k.Bottom,
		k.PageUp,
		k.PageDown,
		k.HalfPageUp,
		k.HalfPageDown,
		k.Home,
		k.Info,
		k.Messages,
		k.Help,
		k.Quit,
	}
}

// FullHelp returns keybindings for the expanded help view.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Home},                               // Navigation
		{k.Top, k.Bottom, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown}, // Pagination
		{k.Select, k.Download, k.Copy, k.Open, k.Edit, k.Refresh, k.Search},   // Actions
		{k.Esc, k.Info, k.Messages, k.Help, k.Quit},                           // App
	}
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Top: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "top"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "bottom"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("ctrl+b"),
		key.WithHelp("ctrl+b", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "page down"),
	),
	HalfPageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "half page up"),
	),
	HalfPageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "half page down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "back"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l", "enter"),
		key.WithHelp("→/l", "enter"),
	),
	Home: key.NewBinding(
		key.WithKeys("H"),
		key.WithHelp("H", "home"),
	),
	Select: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "select"),
	),
	Download: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "download"),
	),
	Copy: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "copy uri"),
	),
	Open: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "refresh"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "clear / close"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Info: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "metadata"),
	),
	Messages: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "messages"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}
