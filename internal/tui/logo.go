package tui

import (
	"fmt"
	"strings"

	"ai-pr-review/internal/q/termformat"
)

var mascotLines = []string{
	`  ▄▄▄                       ▄▄▄  `,
	` ▐█ █▌                     ▐█ █▌ `,
	` ▐███████████████████████████████▌`,
	` █    ▄███▄           ▄███▄    █ `,
	` █    █████           █████    █ `,
	` █    █████           █████    █ `,
	` █    ▀▀▀▀▀           ▀▀▀▀▀    █ `,
	` █                             █ `,
	` █    ──  ▐▄▄▄▄▄▄▄▌  ──       █ `,
	` █        ▐ · ▄▄ · ▌          █ `,
	` █        ▌  ██ ██  ▐         █ `,
	` ▀███████████████████████████▀  `,
	`   ▀▀▀  ▀▀▀▀    ▀▀▀▀  ▀▀▀      `,
}

func RenderLogo(version string) string {
	bodyColor := termformat.ANSI256Color(33)
	dimColor := termformat.ANSI256Color(240)
	divColor := termformat.ANSI256Color(238)

	catStyle := termformat.Style{Foreground: bodyColor}
	nameStyle := termformat.Style{Foreground: bodyColor, Bold: termformat.StyleSetOn}
	verStyle := termformat.Style{Foreground: dimColor}
	tagStyle := termformat.Style{Foreground: dimColor, Italic: termformat.StyleSetOn}
	divStyle := termformat.Style{Foreground: divColor}

	cat := catStyle.Wrap(strings.Join(mascotLines, "\n"))
	name := nameStyle.Wrap("ai-pr-review")
	ver := verStyle.Wrap(" v" + version)
	tag := tagStyle.Wrap("AI PR Review 助手")
	div := divStyle.Wrap(strings.Repeat("─", 24))

	return fmt.Sprintf("%s\n\n  %s%s\n  %s\n  %s\n\n", cat, name, ver, tag, div)
}
