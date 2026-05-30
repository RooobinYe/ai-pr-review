package tui

import "ai-pr-review/internal/q/termformat"

// All TUI styles are derived from currentTheme.
// Call rebuildStyles (via SetTheme) to refresh after a theme switch.
var (
	headerStyle          termformat.Style
	modelTagStyle        termformat.Style
	userLabelStyle       termformat.Style
	assistantLabelStyle  termformat.Style
	toolRunningStyle     termformat.Style
	toolDoneStyle        termformat.Style
	toolFailedStyle      termformat.Style
	statusStyle          termformat.Style
	warnStyle            termformat.Style
	errorStyle           termformat.Style
	helpBoxStyle         termformat.BlockStyle
	slashHintBoxStyle    termformat.BlockStyle
	slashCmdStyle        termformat.Style
	dividerStyle         termformat.Style
	inputPromptStyle     termformat.Style
	pickerHeaderStyle    termformat.Style
	selectedModelStyle   termformat.Style
	unselectedModelStyle termformat.Style
)

func init() { rebuildStyles(currentTheme) }

func rebuildStyles(t Theme) {
	headerStyle = termformat.Style{
		Foreground: t.Primary,
		Bold:       termformat.StyleSetOn,
	}

	modelTagStyle = termformat.Style{
		Foreground: t.Muted,
	}

	userLabelStyle = termformat.Style{
		Foreground: t.UserLabel,
		Bold:       termformat.StyleSetOn,
	}

	assistantLabelStyle = termformat.Style{
		Foreground: t.AssistantLabel,
		Bold:       termformat.StyleSetOn,
	}

	toolRunningStyle = termformat.Style{
		Foreground: t.ToolRunning,
		Italic:     termformat.StyleSetOn,
	}

	toolDoneStyle = termformat.Style{
		Foreground: t.ToolDone,
	}

	toolFailedStyle = termformat.Style{
		Foreground: t.ToolFailed,
		Bold:       termformat.StyleSetOn,
	}

	statusStyle = termformat.Style{
		Foreground: t.Muted,
	}

	warnStyle = termformat.Style{
		Foreground: t.Warning,
	}

	errorStyle = termformat.Style{
		Foreground: t.Error,
	}

	helpBoxStyle = termformat.BlockStyle{
		BorderStyle:      termformat.BorderStyleBasic,
		BorderForeground: t.Secondary,
		Padding:         1,
	}

	slashHintBoxStyle = termformat.BlockStyle{
		BorderStyle:      termformat.BorderStyleBasic,
		BorderForeground: t.Primary,
		Padding:         0,
	}

	slashCmdStyle = termformat.Style{
		Foreground: t.Primary,
		Bold:       termformat.StyleSetOn,
	}

	dividerStyle = termformat.Style{
		Foreground: t.Subtle,
	}

	inputPromptStyle = termformat.Style{
		Foreground: t.InputPrompt,
		Bold:       termformat.StyleSetOn,
	}

	pickerHeaderStyle = termformat.Style{
		Foreground: t.Primary,
		Bold:       termformat.StyleSetOn,
	}

	selectedModelStyle = termformat.Style{
		Foreground: t.SelectedItem,
		Bold:       termformat.StyleSetOn,
	}

	unselectedModelStyle = termformat.Style{
		Foreground: t.UnselectedItem,
	}
}
