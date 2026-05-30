package tuicontrols

import (
	"strings"

	"ai-pr-review/internal/q/termformat"
	"ai-pr-review/internal/q/tui"
	"ai-pr-review/internal/q/uni"
)

// View is a scrollable, fixed-size view over a newline-delimited string.
//
// Invariant:
//   - Offset() is always in the range [0, number of lines).
//   - Empty content counts as 1 line.
type View struct {
	width                    int
	height                   int
	offset                   int
	content                  string
	lines                    []string
	keyMap                   *KeyMap
	emptyLineBackgroundColor termformat.Color
}

// NewView returns a new view of the given size.
func NewView(width, height int) *View {
	v := &View{
		width:  max0(width),
		height: max0(height),
	}
	v.lines = splitLinesToWidth(v.content, v.width)

	km := NewKeyMap()
	km.Add(tui.KeyEvent{ControlKey: tui.ControlKeyPageUp}, "pageup")
	km.Add(tui.KeyEvent{ControlKey: tui.ControlKeyPageDown}, "pagedown")
	km.Add(tui.KeyEvent{ControlKey: tui.ControlKeyUp}, "up")
	km.Add(tui.KeyEvent{ControlKey: tui.ControlKeyDown}, "down")
	km.Add(tui.KeyEvent{ControlKey: tui.ControlKeyHome}, "home")
	km.Add(tui.KeyEvent{ControlKey: tui.ControlKeyEnd}, "end")
	v.keyMap = km

	return v
}

// Init implements tui.Model's Init.
func (v *View) Init(t *tui.TUI) {}

// Update implements tui.Model's Update.
func (v *View) Update(t *tui.TUI, m tui.Message) {
	if v == nil || v.keyMap == nil {
		return
	}
	switch v.keyMap.Process(m) {
	case "pageup":
		v.PageUp()
	case "pagedown":
		v.PageDown()
	case "up":
		v.ScrollUp(1)
	case "down":
		v.ScrollDown(1)
	case "home":
		v.ScrollToTop()
	case "end":
		v.ScrollToBottom()
	}
}

// View implements tui.Model's View. Renders the content clipped to the view size and current offset.
//
// The rendered output always contains exactly Height() rows, but does not pad lines to Width() cells. Each rendered row contains at most Width() visible cells (after
// accounting for ANSI control codes and character widths).
func (v *View) View() string {
	if v == nil || v.height == 0 {
		return ""
	}
	if v.lines == nil {
		v.lines = splitLinesToWidth(v.content, v.width)
	}

	rows := make([]string, 0, v.height)
	for i := 0; i < v.height; i++ {
		if v.content == "" {
			rows = append(rows, v.renderEmptyRow())
			continue
		}
		lineIdx := v.offset + i
		if lineIdx < 0 || lineIdx >= len(v.lines) {
			rows = append(rows, v.renderEmptyRow())
			continue
		}
		rows = append(rows, v.lines[lineIdx])
	}
	return strings.Join(rows, "\n")
}

// SetSize sets the width and height of the view to w, h. Does not affect Offset(); may affect ScrollPercent.
func (v *View) SetSize(w, h int) {
	if v == nil {
		return
	}
	oldWidth := v.width
	v.width = max0(w)
	v.height = max0(h)
	if v.width != oldWidth {
		v.lines = splitLinesToWidth(v.content, v.width)
		v.clampOffset()
	}
}

// Width returns the width.
func (v *View) Width() int {
	if v == nil {
		return 0
	}
	return v.width
}

// Height returns the height.
func (v *View) Height() int {
	if v == nil {
		return 0
	}
	return v.height
}

// SetEmptyLineBackgroundColor sets the background color for rows that have no content.
func (v *View) SetEmptyLineBackgroundColor(c termformat.Color) {
	if v == nil {
		return
	}
	v.emptyLineBackgroundColor = c
}

// Offset returns the offset of the view in lines (e.g. 0 -> unscrolled; 1 -> scrolled down 1 line).
func (v *View) Offset() int {
	if v == nil {
		return 0
	}
	return v.offset
}

// ScrollPercent returns the scroll percent in [0, 100]. 0 means the first line is visible. 100 means the last line is fully visible.
//
// If the last line is visible and the first line is not visible, ScrollPercent returns 100 even if the view's height is greater than the number of content lines.
func (v *View) ScrollPercent() int {
	if v == nil {
		return 0
	}

	numLines := len(v.lines)
	if numLines == 0 {
		return 0
	}

	if v.height <= 0 {
		return 0
	}

	if v.AtTop() && v.AtBottom() {
		return 0
	}
	if v.AtBottom() {
		return 100
	}

	maxOffset := v.maxOffset()
	if maxOffset <= 0 {
		return 0
	}

	if v.offset <= 0 {
		return 0
	}
	if v.offset >= maxOffset {
		return 100
	}
	return (v.offset * 100) / maxOffset
}

// ScrollUp scrolls up n lines.
func (v *View) ScrollUp(n int) {
	if v == nil || n <= 0 {
		return
	}
	v.offset -= n
	if v.offset < 0 {
		v.offset = 0
	}
}

// ScrollDown scrolls down n lines.
func (v *View) ScrollDown(n int) {
	if v == nil || n <= 0 {
		return
	}
	v.offset += n
	v.clampOffset()
	v.normalizeOffset()
}

// PageUp scrolls up one page: ScrollUp(max(1, v.Height()-1)).
func (v *View) PageUp() {
	if v == nil {
		return
	}
	n := v.height - 1
	if n < 1 {
		n = 1
	}
	v.ScrollUp(n)
}

// PageDown scrolls down one page: ScrollDown(max(1, v.Height()-1)).
func (v *View) PageDown() {
	if v == nil {
		return
	}
	n := v.height - 1
	if n < 1 {
		n = 1
	}
	v.ScrollDown(n)
}

// ScrollToTop sets the offset to 0.
func (v *View) ScrollToTop() {
	if v == nil {
		return
	}
	v.offset = 0
}

// ScrollToBottom scrolls to the bottom, and normalizes the offset so that the most lines possible are visible.
func (v *View) ScrollToBottom() {
	if v == nil {
		return
	}
	v.clampOffset()
	v.offset = v.maxOffset()
}

// AtTop returns true if the view is showing the first line.
func (v *View) AtTop() bool {
	if v == nil {
		return true
	}
	return v.offset <= 0
}

// AtBottom returns true if the view is showing the last line.
func (v *View) AtBottom() bool {
	if v == nil {
		return true
	}
	numLines := len(v.lines)
	if numLines == 0 {
		return true
	}
	if v.height <= 0 {
		return false
	}
	return v.offset+v.height >= numLines
}

// SetContent sets the content to s. This won't change Offset() unless it violates the offset invariant.
func (v *View) SetContent(s string) {
	if v == nil {
		return
	}
	v.content = s
	v.lines = splitLinesToWidth(s, v.width)
	v.clampOffset()
}

func (v *View) clampOffset() {
	if v == nil {
		return
	}
	if v.lines == nil {
		v.lines = splitLinesToWidth(v.content, v.width)
	}
	if v.offset < 0 {
		v.offset = 0
		return
	}
	if len(v.lines) == 0 {
		v.offset = 0
		return
	}
	if v.offset >= len(v.lines) {
		v.offset = len(v.lines) - 1
	}
}

func (v *View) normalizeOffset() {
	if v == nil {
		return
	}
	v.clampOffset()

	maxOffset := v.maxOffset()
	if v.offset > maxOffset {
		v.offset = maxOffset
	}
}

func (v *View) maxOffset() int {
	if v == nil || len(v.lines) == 0 {
		return 0
	}
	if v.height <= 0 {
		return len(v.lines) - 1
	}
	maxOffset := len(v.lines) - v.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if maxOffset > len(v.lines)-1 {
		maxOffset = len(v.lines) - 1
	}
	return maxOffset
}

func (v *View) renderEmptyRow() string {
	if v == nil {
		return ""
	}
	if v.width <= 0 {
		return ""
	}
	if v.emptyLineBackgroundColor == nil {
		return ""
	}
	bg := v.emptyLineBackgroundColor.ANSISequence(true)
	if bg == "" {
		return ""
	}
	return bg + strings.Repeat(" ", v.width) + termformat.ANSIReset
}

func splitLinesToWidth(s string, width int) []string {
	if s == "" {
		return []string{""}
	}
	if width <= 0 {
		return strings.Split(s, "\n")
	}
	logicalLines := strings.Split(s, "\n")
	var out []string
	for _, line := range logicalLines {
		wrapped := wrapLineToDisplayWidth(line, width)
		out = append(out, wrapped...)
	}
	return out
}

// wrapLineToDisplayWidth wraps a single logical line (no newlines) into display lines fitting within width cells.
// ANSI SGR styling is carried forward across wrapped segments.
func wrapLineToDisplayWidth(line string, width int) []string {
	if line == "" {
		return []string{""}
	}
	if width <= 0 {
		return []string{line}
	}

	var out []string
	var builder strings.Builder
	currentWidth := 0
	activeSGR := ""

	flush := func() {
		s := builder.String()
		if activeSGR != "" && !strings.HasSuffix(s, termformat.ANSIReset) {
			s += termformat.ANSIReset
		}
		out = append(out, s)
		builder.Reset()
		if activeSGR != "" {
			builder.WriteString(activeSGR)
		}
		currentWidth = 0
	}

	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			seqLen := ansiSequenceLengthForView(line[i:])
			if seqLen == 0 {
				seqLen = 1
			}
			seq := line[i : i+seqLen]
			builder.WriteString(seq)
			activeSGR = trackActiveSGR(activeSGR, seq)
			i += seqLen
			continue
		}

		nextEsc := strings.IndexByte(line[i:], '\x1b')
		segmentEnd := len(line)
		if nextEsc >= 0 {
			segmentEnd = i + nextEsc
		}
		segment := line[i:segmentEnd]
		i = segmentEnd

		iter := uni.NewGraphemeIterator(segment, nil)
		for iter.Next() {
			grapheme := segment[iter.Start():iter.End()]
			gw := iter.TextWidth()

			if currentWidth+gw > width && builder.Len() > 0 {
				flush()
			}

			builder.WriteString(grapheme)
			currentWidth += gw
		}
	}

	if builder.Len() > 0 {
		s := builder.String()
		if activeSGR != "" && !strings.HasSuffix(s, termformat.ANSIReset) {
			s += termformat.ANSIReset
		}
		out = append(out, s)
	} else if len(out) == 0 {
		out = []string{""}
	}

	return out
}

func ansiSequenceLengthForView(s string) int {
	if len(s) == 0 || s[0] != '\x1b' {
		return 0
	}
	if len(s) == 1 {
		return 1
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
		}
		return 0
	case ']':
		for i := 2; i < len(s); i++ {
			if s[i] == '\a' {
				return i + 1
			}
			if s[i] == '\\' && s[i-1] == '\x1b' {
				return i + 1
			}
		}
		return 0
	case 'P', '^', '_':
		for i := 2; i < len(s); i++ {
			if s[i] == '\\' && s[i-1] == '\x1b' {
				return i + 1
			}
		}
		return 0
	default:
		return 2
	}
}

// trackActiveSGR returns the cumulative active SGR sequence after applying seq.
func trackActiveSGR(current, seq string) string {
	if !strings.HasPrefix(seq, "\x1b[") || !strings.HasSuffix(seq, "m") {
		return current
	}
	params := seq[2 : len(seq)-1]
	if params == "0" || params == "" {
		return ""
	}
	// For simplicity, rebuild the active SGR by appending params.
	// A full SGR state machine would be more accurate but this handles
	// the common case of color/style sequences in streamed text.
	if current == "" {
		return seq
	}
	return current[:len(current)-1] + ";" + params + "m"
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
