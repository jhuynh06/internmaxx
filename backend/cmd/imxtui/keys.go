package main

import "github.com/charmbracelet/bubbles/key"

type keymap struct {
	Up      key.Binding
	Down    key.Binding
	Home    key.Binding
	End     key.Binding
	Yank    key.Binding
	Filter  key.Binding
	Refresh key.Binding
	Days    key.Binding
	Help    key.Binding
	Quit    key.Binding
}

func defaultKeys() keymap {
	return keymap{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Home:    key.NewBinding(key.WithKeys("g", "home")),
		End:     key.NewBinding(key.WithKeys("G", "end")),
		Yank:    key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank url")),
		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Days:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "days")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
