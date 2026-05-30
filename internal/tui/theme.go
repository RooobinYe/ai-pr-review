package tui

import "ai-pr-review/internal/q/termformat"

// Theme defines the named color tokens used throughout the TUI.
// All termformat styles are derived from the active theme.
type Theme struct {
	Primary        termformat.Color
	Secondary      termformat.Color
	Success        termformat.Color
	Warning        termformat.Color
	Error          termformat.Color
	Muted          termformat.Color
	Subtle         termformat.Color
	UserLabel      termformat.Color
	AssistantLabel termformat.Color
	ToolRunning    termformat.Color
	ToolDone       termformat.Color
	ToolFailed     termformat.Color
	InputPrompt    termformat.Color
	SelectedItem   termformat.Color
	UnselectedItem termformat.Color
}

// DarkTheme is the default color scheme for dark-background terminals.
var DarkTheme = Theme{
	Primary:        termformat.ANSI256Color(205),
	Secondary:      termformat.ANSI256Color(62),
	Success:        termformat.ANSI256Color(82),
	Warning:        termformat.ANSI256Color(214),
	Error:          termformat.ANSI256Color(196),
	Muted:          termformat.ANSI256Color(240),
	Subtle:         termformat.ANSI256Color(238),
	UserLabel:      termformat.ANSI256Color(33),
	AssistantLabel: termformat.ANSI256Color(82),
	ToolRunning:    termformat.ANSI256Color(214),
	ToolDone:       termformat.ANSI256Color(240),
	ToolFailed:     termformat.ANSI256Color(196),
	InputPrompt:    termformat.ANSI256Color(33),
	SelectedItem:   termformat.ANSI256Color(170),
	UnselectedItem: termformat.ANSI256Color(252),
}

// LightTheme is a color scheme for light-background terminals.
var LightTheme = Theme{
	Primary:        termformat.ANSI256Color(125),
	Secondary:      termformat.ANSI256Color(61),
	Success:        termformat.ANSI256Color(28),
	Warning:        termformat.ANSI256Color(130),
	Error:          termformat.ANSI256Color(160),
	Muted:          termformat.ANSI256Color(246),
	Subtle:         termformat.ANSI256Color(250),
	UserLabel:      termformat.ANSI256Color(25),
	AssistantLabel: termformat.ANSI256Color(28),
	ToolRunning:    termformat.ANSI256Color(130),
	ToolDone:       termformat.ANSI256Color(246),
	ToolFailed:     termformat.ANSI256Color(160),
	InputPrompt:    termformat.ANSI256Color(25),
	SelectedItem:   termformat.ANSI256Color(125),
	UnselectedItem: termformat.ANSI256Color(236),
}

var currentTheme = DarkTheme

// SetTheme sets the active theme and rebuilds all derived styles.
func SetTheme(t Theme) {
	currentTheme = t
	rebuildStyles(t)
}
