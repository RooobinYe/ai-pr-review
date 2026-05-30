package tui

import (
	"ai-pr-review/internal/auth"
	"ai-pr-review/internal/config"
	"ai-pr-review/internal/q/termformat"
	"ai-pr-review/internal/q/tui"
	"ai-pr-review/internal/q/tui/tuicontrols"
	"ai-pr-review/internal/runtime"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	appVersion   = "0.1.0"
	textareaRows = 3
)

type modelEntry struct {
	id   string
	desc string
}

var anthropicModels = []modelEntry{
	{"claude-opus-4-6", "Most capable — complex reasoning and analysis"},
	{"claude-sonnet-4-6", "Balanced — great performance at speed"},
	{"claude-haiku-4-5-20251001", "Fast and lightweight — quick tasks"},
}

var openAIModels = []modelEntry{
	{"gpt-4o", "Most capable — multimodal tasks and analysis"},
	{"gpt-4o-mini", "Fast and affordable — everyday tasks"},
	{"o1-mini", "Reasoning model — math and logic"},
}

type loginProviderEntry struct {
	id   string
	name string
	desc string
}

var loginProviders = []loginProviderEntry{
	{"anthropic", "Anthropic", "Claude Sonnet, Opus, Haiku models"},
	{"openai", "OpenAI", "GPT-4o and GPT-4o-mini models"},
}

type loginMethodEntry struct {
	id   string
	name string
	desc string
}

var anthropicAuthMethods = []loginMethodEntry{
	{"oauth", "OAuth (browser)", "Log in with your Claude.ai account"},
	{"api_key", "API Key", "Enter your Anthropic API key manually"},
}

type slashCommand struct {
	cmd  string
	desc string
}

var slashCommands = []slashCommand{
	{"/model", "Change the active model"},
	{"/help", "Show help and key bindings"},
	{"/login", "Login to AI provider"},
	{"/clear", "Clear conversation history"},
	{"/status", "Show session status"},
	{"/config", "Show or set configuration"},
	{"/session", "List, save, or load sessions"},
	{"/session-list", "List saved sessions"},
	{"/init", "Create project config file"},
	{"/cost", "Show token usage and cost"},
	{"/theme", "Switch dark/light theme"},
	{"/auth", "Auth login, logout, status"},
	{"/exit", "Exit the application"},
	{"/quit", "Exit the application"},
}

type appState int

const (
	stateInput          appState = iota
	stateBusy
	statePicker
	stateHelp
	statePermission
	stateLoginProvider
	stateLoginMethod
	stateLoginAPIKey
	stateLoginOAuth
	stateAskUser
)

// Custom message types for the TUI event loop.
type (
	streamDeltaMsg    struct{ text string }
	streamToolMsg     struct{ name, input string }
	streamToolDoneMsg struct{ name, result string }
	streamUsageMsg    struct{ inputTokens, outputTokens int }
	streamDoneMsg     struct{}
	streamErrMsg      struct{ err error }
	streamWarnMsg     struct{ text string }
	streamPermAskMsg  struct {
		name, input string
		reply       chan runtime.PermDecision
	}
	streamAskUserMsg struct {
		question string
		reply    chan string
	}
	loginCompleteMsg struct {
		provider string
		token    string
		method   string
		err      error
	}
	spinnerTickMsg struct{}
)

type Model struct {
	state  appState
	width  int
	height int
	ready  bool

	viewport *tuicontrols.View
	textarea *tuicontrols.TextArea

	history *inputHistory

	pickerCursor int

	viewBuf   string
	streamBuf string

	inputTokens  int
	outputTokens int

	hasStreamContent bool

	streamChan chan runtime.TurnEvent

	permToolName  string
	permToolInput string
	permReplyCh   chan runtime.PermDecision

	askUserQuestion string
	askUserReplyCh  chan string
	askUserInput    string

	loginCursor   int
	loginProvider string
	loginKeyInput string

	spinnerFrame int

	slashHints string

	loop *runtime.ConversationLoop
	cfg  *runtime.Config
	tui  *tui.TUI
}

func NewModel(cfg *runtime.Config, loop *runtime.ConversationLoop) *Model {
	ta := tuicontrols.NewTextArea(80, textareaRows)
	ta.Placeholder = "Type a message or /help..."

	return &Model{
		state:    stateInput,
		textarea: ta,
		history:  newInputHistory(),
		loop:     loop,
		cfg:      cfg,
		viewBuf:  RenderLogo(appVersion),
	}
}

func (m *Model) Init(t *tui.TUI) {
	m.tui = t
	_ = t.SendPeriodically(spinnerTickMsg{}, 100*time.Millisecond)
}

func (m *Model) Update(t *tui.TUI, msg tui.Message) {
	m.tui = t

	switch msg := msg.(type) {
	case tui.ResizeEvent:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.ready = true
			m.initViewport()
		} else {
			m.resizeViewport()
		}

	case tui.KeyEvent:
		m.handleKey(msg)

	case tui.MouseEvent:
		if msg.IsWheel() && m.viewport != nil {
			switch msg.Button {
			case tui.MouseButtonWheelUp:
				m.viewport.ScrollUp(3)
			case tui.MouseButtonWheelDown:
				m.viewport.ScrollDown(3)
			}
		}

	case streamDeltaMsg:
		if !m.hasStreamContent {
			m.hasStreamContent = true
		}
		m.streamBuf += msg.text
		m.refreshViewport()

	case streamToolMsg:
		if !m.hasStreamContent {
			m.hasStreamContent = true
		}
		line := toolRunningStyle.Wrap(fmt.Sprintf("  ◆ %s: %s\n", msg.name, truncate(msg.input, 60)))
		m.streamBuf += line
		m.refreshViewport()

	case streamToolDoneMsg:
		suffix := ""
		if msg.result != "" {
			suffix = " → " + truncate(msg.result, 40)
		}
		line := toolDoneStyle.Wrap(fmt.Sprintf("  ✓ %s%s\n", msg.name, suffix))
		m.streamBuf += line
		m.refreshViewport()

	case streamUsageMsg:
		m.inputTokens = msg.inputTokens
		m.outputTokens = msg.outputTokens

	case streamDoneMsg:
		if m.streamBuf != "" || m.hasStreamContent {
			tokLine := statusStyle.Wrap(fmt.Sprintf(
				"\n\nTokens: %s in / %s out\n\n",
				formatNum(m.inputTokens),
				formatNum(m.outputTokens),
			))
			m.viewBuf += m.streamBuf + tokLine
			m.streamBuf = ""
		}
		m.hasStreamContent = false
		m.state = stateInput
		m.refreshViewport()
		m.viewport.ScrollToBottom()

	case streamWarnMsg:
		m.viewBuf += warnStyle.Wrap(fmt.Sprintf("Warning: %s\n\n", msg.text))
		m.refreshViewport()

	case streamPermAskMsg:
		m.state = statePermission
		m.permToolName = msg.name
		m.permToolInput = msg.input
		m.permReplyCh = msg.reply

	case streamAskUserMsg:
		m.askUserInput = ""
		m.askUserQuestion = msg.question
		m.askUserReplyCh = msg.reply
		m.state = stateAskUser

	case streamErrMsg:
		m.viewBuf += errorStyle.Wrap(fmt.Sprintf("Error: %v\n\n", msg.err))
		m.streamBuf = ""
		m.hasStreamContent = false
		m.state = stateInput
		m.refreshViewport()

	case loginCompleteMsg:
		m.handleLoginComplete(msg)

	case spinnerTickMsg:
		m.spinnerFrame++
	}
}

func (m *Model) View() string {
	if !m.ready {
		return "Initialising...\n"
	}

	switch m.state {
	case statePicker:
		return m.viewPicker()
	case stateHelp:
		return m.viewHelp()
	case statePermission:
		return m.viewPermission()
	case stateAskUser:
		return m.viewAskUser()
	case stateLoginProvider:
		return m.viewLoginProvider()
	case stateLoginMethod:
		return m.viewLoginMethod()
	case stateLoginAPIKey:
		return m.viewLoginAPIKey()
	case stateLoginOAuth:
		return m.viewLoginOAuth()
	}

	header := m.renderHeader()
	divider := dividerStyle.Wrap(strings.Repeat("─", m.width))
	hint := statusStyle.Wrap("Enter=send  Ctrl+J=newline  ↑↓=history  Tab=autocomplete")
	statusLine := m.renderStatusBar()
	inputArea := m.renderInputArea()

	vpOutput := m.viewport.View()
	if m.slashHints != "" {
		vpOutput = m.overlaySlashHints(vpOutput)
	}

	return strings.Join([]string{
		header,
		vpOutput,
		divider,
		inputArea,
		hint,
		statusLine,
	}, "\n")
}

func (m *Model) handleKey(msg tui.KeyEvent) {
	switch m.state {
	case statePicker:
		m.handlePickerKey(msg)
		return
	case stateHelp:
		m.handleHelpKey(msg)
		return
	case statePermission:
		m.handlePermissionKey(msg)
		return
	case stateAskUser:
		m.handleAskUserKey(msg)
		return
	case stateLoginProvider:
		m.handleLoginProviderKey(msg)
		return
	case stateLoginMethod:
		m.handleLoginMethodKey(msg)
		return
	case stateLoginAPIKey:
		m.handleLoginAPIKeyKey(msg)
		return
	case stateLoginOAuth:
		m.handleLoginOAuthKey(msg)
		return
	case stateBusy:
		if msg.ControlKey == tui.ControlKeyCtrlC {
			m.tui.Quit()
		}
		return
	}

	// stateInput
	switch msg.ControlKey {
	case tui.ControlKeyCtrlC:
		m.tui.Quit()

	case tui.ControlKeyEnter:
		m.handleSubmit()

	case tui.ControlKeyCtrlJ:
		m.history.Reset()
		m.textarea.Update(m.tui, msg)

	case tui.ControlKeyUp:
		if !strings.Contains(m.textarea.Contents(), "\n") {
			prev := m.history.Prev(m.textarea.Contents())
			m.textarea.SetContents(prev)
			m.updateSlashHints()
			return
		}
		m.textarea.Update(m.tui, msg)

	case tui.ControlKeyDown:
		if !strings.Contains(m.textarea.Contents(), "\n") {
			next := m.history.Next()
			m.textarea.SetContents(next)
			m.updateSlashHints()
			return
		}
		m.textarea.Update(m.tui, msg)

	case tui.ControlKeyTab:
		text := strings.TrimSpace(m.textarea.Contents())
		if strings.HasPrefix(text, "/") && !strings.Contains(text, " ") {
			m.history.Reset()
			m.autocompleteSlash(text)
			m.updateSlashHints()
			return
		}
		m.history.Reset()
		m.textarea.Update(m.tui, msg)

	default:
		m.history.Reset()
		m.textarea.Update(m.tui, msg)
	}

	m.updateSlashHints()
}

func (m *Model) handleSubmit() {
	text := strings.TrimSpace(m.textarea.Contents())
	if text == "" {
		return
	}
	m.textarea.SetContents("")
	m.history.Push(text)
	m.history.Reset()
	m.slashHints = ""

	if strings.HasPrefix(text, "/") {
		m.handleSlashCommand(text)
		return
	}
	m.startMessage(text)
}

func (m *Model) handleSlashCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/model":
		m.state = statePicker
		m.pickerCursor = 0
		for i, km := range m.activeModels() {
			if km.id == m.cfg.Model {
				m.pickerCursor = i
				break
			}
		}

	case "/help":
		m.state = stateHelp

	case "/login":
		m.state = stateLoginProvider
		m.loginCursor = 0

	case "/clear":
		m.loop.ClearSession()
		m.viewBuf = statusStyle.Wrap("Session cleared.\n\n")
		m.streamBuf = ""
		m.inputTokens = 0
		m.outputTokens = 0
		m.refreshViewport()

	case "/session-list":
		metas, err := m.loop.ListSessionsWithMeta()
		if err != nil {
			m.viewBuf += errorStyle.Wrap(fmt.Sprintf("Error listing sessions: %v\n\n", err))
		} else if len(metas) == 0 {
			m.viewBuf += statusStyle.Wrap("No saved sessions.\n\n")
		} else {
			m.viewBuf += statusStyle.Wrap(formatSessionList(metas) + "\n\n")
		}
		m.refreshViewport()

	case "/theme":
		theme := "dark"
		if len(parts) > 1 {
			theme = parts[1]
		}
		switch theme {
		case "light":
			SetTheme(LightTheme)
			m.viewBuf += statusStyle.Wrap("Theme: light.\n\n")
		default:
			SetTheme(DarkTheme)
			m.viewBuf += statusStyle.Wrap("Theme: dark.\n\n")
		}
		m.refreshViewport()

	case "/auth":
		sub := "status"
		if len(parts) > 1 {
			sub = parts[1]
		}
		msg := m.handleAuthSubcommand(sub)
		m.viewBuf += statusStyle.Wrap(msg + "\n\n")
		m.refreshViewport()

	case "/session":
		m.handleSessionCommand(parts)

	case "/status":
		m.handleStatus()

	case "/init":
		m.handleInit()

	case "/cost":
		m.handleCost()

	case "/config":
		m.handleConfig(parts)

	case "/exit", "/quit":
		m.tui.Quit()

	default:
		m.viewBuf += errorStyle.Wrap(fmt.Sprintf("Unknown command: %s  (type /help for commands)\n\n", parts[0]))
		m.refreshViewport()
	}
}

func (m *Model) handleSessionCommand(parts []string) {
	sub := "list"
	if len(parts) > 1 {
		sub = parts[1]
	}
	switch sub {
	case "list":
		metas, err := m.loop.ListSessionsWithMeta()
		if err != nil {
			m.viewBuf += errorStyle.Wrap(fmt.Sprintf("Error listing sessions: %v\n\n", err))
		} else if len(metas) == 0 {
			m.viewBuf += statusStyle.Wrap("No saved sessions.\n\n")
		} else {
			m.viewBuf += statusStyle.Wrap(formatSessionList(metas) + "\n\n")
		}
	case "save":
		name := ""
		if len(parts) > 2 {
			name = parts[2]
		}
		if name != "" {
			m.loop.Session.ID = name
		}
		if err := m.loop.SaveCurrentSession(); err != nil {
			m.viewBuf += errorStyle.Wrap(fmt.Sprintf("Error saving session: %v\n\n", err))
		} else {
			m.viewBuf += statusStyle.Wrap(fmt.Sprintf("Session saved: %s\n\n", m.loop.Session.ID))
		}
	case "load":
		if len(parts) < 3 {
			m.viewBuf += errorStyle.Wrap("Usage: /session load <name>\n\n")
		} else {
			id := parts[2]
			if err := m.loop.LoadNamedSession(id); err != nil {
				m.viewBuf += errorStyle.Wrap(fmt.Sprintf("Error loading session %q: %v\n\n", id, err))
			} else {
				m.viewBuf += statusStyle.Wrap(fmt.Sprintf("Session loaded: %s (%d messages)\n\n", id, m.loop.MessageCount()))
			}
		}
	default:
		m.viewBuf += errorStyle.Wrap(fmt.Sprintf("Unknown /session subcommand %q. Usage: /session list|save|load <name>\n\n", sub))
	}
	m.refreshViewport()
}

func (m *Model) handleStatus() {
	permMode := "default"
	if m.cfg.PermissionMode != "" {
		permMode = m.cfg.PermissionMode
	}
	if m.loop.PermManager != nil {
		permMode = m.loop.PermManager.Mode.String()
	}
	lines := []string{
		fmt.Sprintf("Provider       : %s", m.cfg.ProviderName),
		fmt.Sprintf("Model          : %s", m.cfg.Model),
		fmt.Sprintf("Permission mode: %s", permMode),
		fmt.Sprintf("Session ID     : %s", m.loop.Session.ID),
		fmt.Sprintf("Messages       : %d", m.loop.MessageCount()),
		fmt.Sprintf("Tokens in/out  : %s / %s", formatNum(m.inputTokens), formatNum(m.outputTokens)),
	}
	m.viewBuf += statusStyle.Wrap(strings.Join(lines, "\n") + "\n\n")
	m.refreshViewport()
}

func (m *Model) handleInit() {
	err := config.InitProject(m.cfg.Model)
	switch {
	case err == nil:
		m.viewBuf += statusStyle.Wrap("Created .ai-pr-review/settings.json with defaults.\n\n")
	case os.IsExist(err):
		m.viewBuf += statusStyle.Wrap(".ai-pr-review/settings.json already exists — no changes made.\n\n")
	default:
		m.viewBuf += errorStyle.Wrap(fmt.Sprintf("init: %v\n\n", err))
	}
	m.refreshViewport()
}

func (m *Model) handleCost() {
	var report string
	if m.loop.Usage != nil && m.loop.Usage.Turns > 0 {
		report = m.loop.Usage.FormatSummary()
		if m.loop.Compaction.CompactionCount > 0 {
			report += fmt.Sprintf("Compactions    : %d\n", m.loop.Compaction.CompactionCount)
		}
	} else {
		c := m.loop.Compaction
		lines := []string{
			fmt.Sprintf("Input tokens   : %s", formatNum(c.TotalInputTokens)),
			fmt.Sprintf("Output tokens  : %s", formatNum(c.TotalOutputTokens)),
			fmt.Sprintf("Compactions    : %d", c.CompactionCount),
			"Cost           : unavailable (no completed turns yet)",
		}
		report = strings.Join(lines, "\n")
	}
	m.viewBuf += statusStyle.Wrap(report + "\n\n")
	m.refreshViewport()
}

func (m *Model) handleConfig(parts []string) {
	if len(parts) == 1 {
		permMode := m.cfg.PermissionMode
		if permMode == "" {
			permMode = "default"
		}
		lines := []string{
			fmt.Sprintf("model          = %s", m.cfg.Model),
			fmt.Sprintf("apiKey         = %s", maskString(m.cfg.APIKey)),
			fmt.Sprintf("baseURL        = %s", m.cfg.BaseURL),
			fmt.Sprintf("permissionMode = %s", permMode),
			fmt.Sprintf("maxTokens      = %d", m.cfg.MaxTokens),
			fmt.Sprintf("theme          = %s", m.cfg.Theme),
		}
		m.viewBuf += statusStyle.Wrap(strings.Join(lines, "\n") + "\n\n")
		m.refreshViewport()
		return
	}

	key := parts[1]
	if len(parts) == 2 {
		val := m.configGet(key)
		if val == "" {
			m.viewBuf += errorStyle.Wrap(fmt.Sprintf("Unknown config key: %s\n\n", key))
		} else {
			m.viewBuf += statusStyle.Wrap(fmt.Sprintf("%s = %s\n\n", key, val))
		}
		m.refreshViewport()
		return
	}

	value := strings.Join(parts[2:], " ")
	if err := m.configSet(key, value); err != nil {
		m.viewBuf += errorStyle.Wrap(fmt.Sprintf("config set: %v\n\n", err))
	} else {
		m.viewBuf += statusStyle.Wrap(fmt.Sprintf("Set %s = %s\n\n", key, value))
	}
	m.refreshViewport()
}

func (m *Model) configGet(key string) string {
	switch key {
	case "model":
		return m.cfg.Model
	case "apiKey":
		return maskString(m.cfg.APIKey)
	case "baseURL":
		return m.cfg.BaseURL
	case "permissionMode":
		if m.cfg.PermissionMode == "" {
			return "default"
		}
		return m.cfg.PermissionMode
	case "maxTokens":
		return fmt.Sprintf("%d", m.cfg.MaxTokens)
	case "theme":
		return m.cfg.Theme
	default:
		return ""
	}
}

func (m *Model) configSet(key, value string) error {
	s := &config.Settings{}
	switch key {
	case "model":
		m.cfg.Model = value
		m.loop.Config.Model = value
		s.Model = value
	case "apiKey":
		m.cfg.APIKey = value
		s.APIKey = value
	case "baseURL":
		m.cfg.BaseURL = value
		s.BaseURL = value
	case "permissionMode":
		m.cfg.PermissionMode = value
		s.PermissionMode = value
	case "theme":
		m.cfg.Theme = value
		s.Theme = value
		switch value {
		case "light":
			SetTheme(LightTheme)
		default:
			SetTheme(DarkTheme)
		}
	default:
		return fmt.Errorf("unknown config key %q (valid: model, permissionMode, theme)", key)
	}
	return config.WriteProject(s)
}

func (m *Model) handleAuthSubcommand(sub string) string {
	switch sub {
	case "login":
		td, err := auth.StartOAuthFlow()
		if err != nil {
			return fmt.Sprintf("Auth login error: %v", err)
		}
		if err := auth.SaveTokens(td); err != nil {
			return fmt.Sprintf("Login succeeded but could not save token: %v", err)
		}
		return "Login successful. Token saved to ~/.ai-pr-review/auth.json"

	case "logout":
		if err := auth.ClearTokens(); err != nil {
			return fmt.Sprintf("Logout error: %v", err)
		}
		return "Logged out. Stored tokens cleared."

	case "status":
		s := auth.GetStatus()
		lines := []string{
			fmt.Sprintf("Provider       : %s", m.cfg.ProviderName),
			fmt.Sprintf("Authenticated  : %v", s.Authenticated),
			fmt.Sprintf("Method         : %s", s.Method),
		}
		if s.Method == "oauth" && !s.ExpiresAt.IsZero() {
			lines = append(lines,
				fmt.Sprintf("Token expires  : %s", s.ExpiresAt.Format("2006-01-02 15:04:05 MST")),
				fmt.Sprintf("Has refresh    : %v", s.HasRefresh),
			)
		}
		return strings.Join(lines, "\n")

	default:
		return fmt.Sprintf("Unknown auth subcommand %q. Usage: /auth login | logout | status", sub)
	}
}

func (m *Model) handleLoginProviderKey(msg tui.KeyEvent) {
	switch msg.ControlKey {
	case tui.ControlKeyCtrlC:
		m.tui.Quit()
	case tui.ControlKeyEsc:
		m.state = stateInput
	case tui.ControlKeyUp:
		if m.loginCursor > 0 {
			m.loginCursor--
		}
	case tui.ControlKeyDown:
		if m.loginCursor < len(loginProviders)-1 {
			m.loginCursor++
		}
	case tui.ControlKeyEnter:
		chosen := loginProviders[m.loginCursor]
		m.loginProvider = chosen.id
		m.loginCursor = 0
		switch chosen.id {
		case "anthropic":
			m.state = stateLoginMethod
		default:
			m.startAPIKeyInput()
		}
	}
}

func (m *Model) handleLoginMethodKey(msg tui.KeyEvent) {
	switch msg.ControlKey {
	case tui.ControlKeyCtrlC:
		m.tui.Quit()
	case tui.ControlKeyEsc:
		m.state = stateLoginProvider
		m.loginCursor = 0
	case tui.ControlKeyUp:
		if m.loginCursor > 0 {
			m.loginCursor--
		}
	case tui.ControlKeyDown:
		if m.loginCursor < len(anthropicAuthMethods)-1 {
			m.loginCursor++
		}
	case tui.ControlKeyEnter:
		chosen := anthropicAuthMethods[m.loginCursor]
		m.loginCursor = 0
		switch chosen.id {
		case "oauth":
			m.startOAuthLogin()
		default:
			m.startAPIKeyInput()
		}
	}
}

func (m *Model) startAPIKeyInput() {
	m.loginKeyInput = ""
	m.state = stateLoginAPIKey
}

func (m *Model) handleLoginAPIKeyKey(msg tui.KeyEvent) {
	switch msg.ControlKey {
	case tui.ControlKeyCtrlC:
		m.tui.Quit()
	case tui.ControlKeyEsc:
		m.state = stateInput
	case tui.ControlKeyEnter:
		apiKey := strings.TrimSpace(m.loginKeyInput)
		if apiKey == "" {
			m.viewBuf += errorStyle.Wrap("API key cannot be empty.\n\n")
			m.state = stateInput
			m.refreshViewport()
			return
		}
		saveErr := auth.SetProviderAPIKey(m.loginProvider, apiKey)
		if saveErr != nil {
			m.handleLoginComplete(loginCompleteMsg{err: saveErr})
			return
		}
		m.handleLoginComplete(loginCompleteMsg{
			provider: m.loginProvider,
			token:    apiKey,
			method:   "api_key",
		})
	default:
		if msg.IsRunes() {
			m.loginKeyInput += string(msg.Runes)
		} else if msg.ControlKey == tui.ControlKeyBackspace {
			if len(m.loginKeyInput) > 0 {
				m.loginKeyInput = m.loginKeyInput[:len(m.loginKeyInput)-1]
			}
		}
	}
}

func (m *Model) startOAuthLogin() {
	session, err := auth.PrepareOAuthFlow()
	if err != nil {
		m.viewBuf += errorStyle.Wrap(fmt.Sprintf("OAuth setup failed: %v\n\n", err))
		m.state = stateInput
		m.refreshViewport()
		return
	}

	m.state = stateLoginOAuth
	m.viewBuf += statusStyle.Wrap(fmt.Sprintf(
		"Opening browser for Anthropic OAuth login...\n"+
			"If your browser doesn't open, visit:\n  %s\n\n"+
			"Waiting for callback... (5-minute timeout)\n\n",
		session.AuthURL,
	))
	m.refreshViewport()

	go func() {
		td, err := session.Complete()
		if err != nil {
			m.tui.Send(loginCompleteMsg{err: err})
			return
		}
		if err := auth.SetProviderOAuth("anthropic", td); err != nil {
			m.tui.Send(loginCompleteMsg{err: fmt.Errorf("save token: %w", err)})
			return
		}
		m.tui.Send(loginCompleteMsg{
			provider: "anthropic",
			token:    td.AccessToken,
			method:   "oauth",
		})
	}()
}

func (m *Model) handleLoginOAuthKey(msg tui.KeyEvent) {
	if msg.ControlKey == tui.ControlKeyCtrlC {
		m.tui.Quit()
	}
}

func (m *Model) handleLoginComplete(result loginCompleteMsg) {
	m.state = stateInput

	if result.err != nil {
		m.viewBuf += errorStyle.Wrap(fmt.Sprintf("Login failed: %v\n\n", result.err))
		m.refreshViewport()
		return
	}

	m.cfg.ProviderName = result.provider
	m.cfg.AuthMethod = result.method
	if result.method == "oauth" {
		m.cfg.OAuthToken = result.token
		m.cfg.APIKey = ""
	} else {
		m.cfg.APIKey = result.token
		m.cfg.OAuthToken = ""
	}

	switch result.provider {
	case "openai":
		m.cfg.Model = "gpt-4o"
	default:
		m.cfg.Model = runtime.DefaultModel
	}
	m.loop.Config.Model = m.cfg.Model

	client, err := runtime.NewProviderClient(m.cfg)
	if err != nil {
		m.viewBuf += errorStyle.Wrap(fmt.Sprintf(
			"Login succeeded but could not create provider client: %v\n\n", err))
		m.refreshViewport()
		return
	}
	m.loop.Client = client

	m.viewBuf += statusStyle.Wrap(fmt.Sprintf(
		"Logged in to %s via %s. Model set to %s. Ready!\n\n",
		result.provider, result.method, m.cfg.Model,
	))
	m.refreshViewport()
}

func (m *Model) handleAskUserKey(msg tui.KeyEvent) {
	switch msg.ControlKey {
	case tui.ControlKeyCtrlC:
		m.tui.Quit()
	case tui.ControlKeyEsc:
		ch := m.askUserReplyCh
		m.askUserReplyCh = nil
		m.askUserQuestion = ""
		m.state = stateBusy
		go func() { ch <- "" }()
	case tui.ControlKeyEnter:
		answer := strings.TrimSpace(m.askUserInput)
		ch := m.askUserReplyCh
		m.askUserReplyCh = nil
		m.askUserQuestion = ""
		m.state = stateBusy
		go func() { ch <- answer }()
	default:
		if msg.IsRunes() {
			m.askUserInput += string(msg.Runes)
		} else if msg.ControlKey == tui.ControlKeyBackspace {
			if len(m.askUserInput) > 0 {
				m.askUserInput = m.askUserInput[:len(m.askUserInput)-1]
			}
		}
	}
}

func (m *Model) viewAskUser() string {
	q := m.askUserQuestion
	if len(q) > 200 {
		q = q[:200] + "..."
	}
	inputDisplay := m.askUserInput
	if inputDisplay == "" {
		inputDisplay = " "
	}
	content := strings.Join([]string{
		headerStyle.Wrap("Agent Question"),
		"",
		"  " + q,
		"",
		"  " + inputDisplay,
		"",
		statusStyle.Wrap("  Enter to answer  •  Esc to skip  •  Ctrl+C to quit"),
	}, "\n")
	box := helpBoxStyle.Apply(content)
	return m.centerBox(box)
}

func (m *Model) startMessage(text string) {
	m.viewBuf += userLabelStyle.Wrap("You") + ": " + text + "\n\n"
	m.viewBuf += assistantLabelStyle.Wrap("Claude") + ": "
	m.state = stateBusy
	m.hasStreamContent = false

	ch := make(chan runtime.TurnEvent, 64)
	m.streamChan = ch

	loop := m.loop
	go func() {
		defer close(ch)
		loop.SendMessageStreaming(context.Background(), text, ch) //nolint:errcheck
	}()

	go m.readStream(m.tui, ch)
	m.refreshViewport()
}

func (m *Model) readStream(t *tui.TUI, ch <-chan runtime.TurnEvent) {
	for {
		ev, ok := <-ch
		if !ok {
			t.Send(streamDoneMsg{})
			return
		}
		switch ev.Type {
		case runtime.TurnEventTextDelta:
			t.Send(streamDeltaMsg{text: ev.Text})
		case runtime.TurnEventToolStart:
			t.Send(streamToolMsg{name: ev.ToolName, input: ev.ToolInput})
		case runtime.TurnEventToolDone:
			t.Send(streamToolDoneMsg{name: ev.ToolName, result: ev.ToolResult})
		case runtime.TurnEventUsage:
			t.Send(streamUsageMsg{inputTokens: ev.InputTokens, outputTokens: ev.OutputTokens})
		case runtime.TurnEventDone:
			t.Send(streamDoneMsg{})
		case runtime.TurnEventError:
			t.Send(streamErrMsg{err: ev.Err})
		case runtime.TurnEventPermissionAsk:
			t.Send(streamPermAskMsg{name: ev.ToolName, input: ev.ToolInput, reply: ev.PermReply})
		case runtime.TurnEventAskUser:
			t.Send(streamAskUserMsg{question: ev.ToolInput, reply: ev.AskUserReply})
		}
	}
}

func (m *Model) handlePickerKey(msg tui.KeyEvent) {
	models := m.activeModels()
	switch msg.ControlKey {
	case tui.ControlKeyEsc:
		m.state = stateInput
	case tui.ControlKeyEnter:
		chosen := models[m.pickerCursor]
		m.cfg.Model = chosen.id
		m.loop.Config.Model = chosen.id
		m.viewBuf += statusStyle.Wrap(fmt.Sprintf("Model changed to %s\n\n", chosen.id))
		m.state = stateInput
		m.refreshViewport()
	case tui.ControlKeyUp:
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
	case tui.ControlKeyDown:
		if m.pickerCursor < len(models)-1 {
			m.pickerCursor++
		}
	case tui.ControlKeyCtrlC:
		m.tui.Quit()
	}
}

func (m *Model) handleHelpKey(msg tui.KeyEvent) {
	switch {
	case msg.ControlKey == tui.ControlKeyEsc, msg.ControlKey == tui.ControlKeyEnter, string(msg.Runes) == "q":
		m.state = stateInput
	case msg.ControlKey == tui.ControlKeyCtrlC:
		m.tui.Quit()
	}
}

func (m *Model) handlePermissionKey(msg tui.KeyEvent) {
	var decision runtime.PermDecision
	handled := true

	switch string(msg.Runes) {
	case "y", "Y":
		decision = runtime.PermDecisionAllowOnce
	case "a", "A":
		decision = runtime.PermDecisionAllowAlways
	case "n", "N":
		decision = runtime.PermDecisionDeny
	default:
		if msg.ControlKey == tui.ControlKeyCtrlC {
			m.tui.Quit()
			return
		}
		handled = false
	}

	if !handled {
		return
	}

	ch := m.permReplyCh
	m.permReplyCh = nil
	m.permToolName = ""
	m.permToolInput = ""
	m.state = stateBusy

	go func() { ch <- decision }()
}

func (m *Model) viewPermission() string {
	tool := userLabelStyle.Wrap(m.permToolName)
	inp := m.permToolInput
	if len(inp) > 60 {
		inp = inp[:60] + "..."
	}
	prompt := fmt.Sprintf("Allow %s: %s?", tool, inp)
	hint := statusStyle.Wrap("[y]es-once  [a]lways  [n]o")
	content := strings.Join([]string{
		headerStyle.Wrap("Permission Required"),
		"",
		"  " + prompt,
		"",
		"  " + hint,
	}, "\n")
	box := helpBoxStyle.Apply(content)
	return m.centerBox(box)
}

func (m *Model) viewPicker() string {
	var b strings.Builder
	b.WriteString(pickerHeaderStyle.Wrap("Select Model") + "\n")
	b.WriteString(statusStyle.Wrap("  ↑/↓ navigate  Enter select  Esc cancel") + "\n\n")

	for i, km := range m.activeModels() {
		cursor := "  "
		style := unselectedModelStyle
		if i == m.pickerCursor {
			cursor = "▶ "
			style = selectedModelStyle
		}
		b.WriteString(cursor + style.Wrap(km.id) + "\n")
		b.WriteString("    " + statusStyle.Wrap(km.desc) + "\n")
	}

	box := helpBoxStyle.Apply(b.String())
	return m.centerBox(box)
}

func (m *Model) viewHelp() string {
	content := strings.Join([]string{
		headerStyle.Wrap("AI PR Review — Commands"),
		"",
		statusStyle.Wrap("Auth & Provider"),
		"  " + userLabelStyle.Wrap("/login") + "                          Multi-provider login flow",
		"  " + userLabelStyle.Wrap("/auth") + " login|logout|status       Legacy OAuth commands",
		"",
		statusStyle.Wrap("Model & Config"),
		"  " + userLabelStyle.Wrap("/model") + "                          Change the active model (picker)",
		"  " + userLabelStyle.Wrap("/config") + "                         Show all config values",
		"  " + userLabelStyle.Wrap("/config") + " <key>                   Show one config value",
		"  " + userLabelStyle.Wrap("/config") + " <key> <value>           Set config value (saves to project)",
		"  " + userLabelStyle.Wrap("/init") + "                           Create .ai-pr-review/settings.json",
		"  " + userLabelStyle.Wrap("/theme") + " dark|light               Switch TUI color theme",
		"  " + userLabelStyle.Wrap("/status") + "                         Show model/provider/session info",
		"  " + userLabelStyle.Wrap("/cost") + "                           Show token usage this session",
		"",
		statusStyle.Wrap("Session"),
		"  " + userLabelStyle.Wrap("/clear") + "                          Clear conversation history",
		"  " + userLabelStyle.Wrap("/session") + " list                   List saved sessions",
		"  " + userLabelStyle.Wrap("/session") + " save [name]            Save current session",
		"  " + userLabelStyle.Wrap("/session") + " load <name>            Load a saved session",
		"  " + userLabelStyle.Wrap("/session-list") + "                   Alias for /session list",
		"",
		statusStyle.Wrap("Other"),
		"  " + userLabelStyle.Wrap("/help") + "                           Show this help",
		"  " + userLabelStyle.Wrap("/exit") + " / " + userLabelStyle.Wrap("/quit") + "                     Exit (session auto-saved)",
		"",
		statusStyle.Wrap("Input:"),
		"  " + userLabelStyle.Wrap("Enter") + "          Send message",
		"  " + userLabelStyle.Wrap("Ctrl+J") + "         Insert newline (multi-line input)",
		"  " + userLabelStyle.Wrap("↑ / ↓") + "          Navigate input history (single-line mode)",
		"  " + userLabelStyle.Wrap("Tab") + "            Autocomplete slash commands",
		"  " + userLabelStyle.Wrap("Scroll") + "          Scroll conversation (mouse/trackpad)",
		"  " + userLabelStyle.Wrap("Ctrl+C") + "         Exit",
		"",
		statusStyle.Wrap("Esc / Enter / q to close this panel"),
	}, "\n")
	box := helpBoxStyle.Apply(content)
	return m.centerBox(box)
}

func (m *Model) viewLoginProvider() string {
	var b strings.Builder
	b.WriteString(pickerHeaderStyle.Wrap("Login — Choose Provider") + "\n")
	b.WriteString(statusStyle.Wrap("  ↑/↓ navigate  Enter select  Esc cancel") + "\n\n")
	for i, p := range loginProviders {
		cursor := "  "
		style := unselectedModelStyle
		if i == m.loginCursor {
			cursor = "▶ "
			style = selectedModelStyle
		}
		b.WriteString(cursor + style.Wrap(p.name) + "\n")
		b.WriteString("    " + statusStyle.Wrap(p.desc) + "\n")
	}
	box := helpBoxStyle.Apply(b.String())
	return m.centerBox(box)
}

func (m *Model) viewLoginMethod() string {
	var b strings.Builder
	b.WriteString(pickerHeaderStyle.Wrap("Login — Anthropic Auth Method") + "\n")
	b.WriteString(statusStyle.Wrap("  ↑/↓ navigate  Enter select  Esc back") + "\n\n")
	for i, meth := range anthropicAuthMethods {
		cursor := "  "
		style := unselectedModelStyle
		if i == m.loginCursor {
			cursor = "▶ "
			style = selectedModelStyle
		}
		b.WriteString(cursor + style.Wrap(meth.name) + "\n")
		b.WriteString("    " + statusStyle.Wrap(meth.desc) + "\n")
	}
	box := helpBoxStyle.Apply(b.String())
	return m.centerBox(box)
}

func (m *Model) viewLoginAPIKey() string {
	providerName := m.loginProvider
	if providerName == "" {
		providerName = "provider"
	}
	masked := strings.Repeat("*", len(m.loginKeyInput))
	if masked == "" {
		masked = " "
	}
	content := strings.Join([]string{
		pickerHeaderStyle.Wrap(fmt.Sprintf("Login — %s API Key", providerName)),
		"",
		"  " + statusStyle.Wrap("Paste your API key below (input is masked):"),
		"  " + masked,
		"",
		"  " + statusStyle.Wrap("Enter to confirm  •  Esc to cancel"),
	}, "\n")
	box := helpBoxStyle.Apply(content)
	return m.centerBox(box)
}

func (m *Model) viewLoginOAuth() string {
	spinChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spin := spinChars[m.spinnerFrame%len(spinChars)]
	content := strings.Join([]string{
		pickerHeaderStyle.Wrap("Login — Anthropic OAuth"),
		"",
		"  " + spin + " " + statusStyle.Wrap("Waiting for browser login..."),
		"",
		"  " + statusStyle.Wrap("Complete the login in your browser, then return here."),
		"  " + statusStyle.Wrap("Ctrl+C to quit."),
	}, "\n")
	box := helpBoxStyle.Apply(content)
	return m.centerBox(box)
}

func (m *Model) renderHeader() string {
	title := headerStyle.Wrap("AI PR Review v" + appVersion)
	tag := modelTagStyle.Wrap(fmt.Sprintf("  [%s] %s", m.cfg.ProviderName, m.cfg.Model))
	return title + tag
}

func (m *Model) renderStatusBar() string {
	if m.inputTokens > 0 || m.outputTokens > 0 {
		return statusStyle.Wrap(fmt.Sprintf(
			"Tokens: %s in / %s out  │  Session: %s",
			formatNum(m.inputTokens), formatNum(m.outputTokens),
			m.loop.Session.ID,
		))
	}
	return statusStyle.Wrap("Session: " + m.loop.Session.ID)
}

func (m *Model) renderInputArea() string {
	prompt := inputPromptStyle.Wrap("> ")
	tv := m.textarea.View()
	lines := strings.Split(tv, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		if i == 0 {
			result[i] = prompt + line
		} else {
			result[i] = "  " + line
		}
	}
	return strings.Join(result, "\n")
}

func (m *Model) activeModels() []modelEntry {
	if m.cfg.ProviderName == "openai" {
		return openAIModels
	}
	return anthropicModels
}

func (m *Model) initViewport() {
	vpHeight := m.viewportHeight()
	m.viewport = tuicontrols.NewView(m.width, vpHeight)
	m.viewport.SetContent(m.viewBuf)
	m.viewport.ScrollToBottom()
	m.textarea.SetSize(m.width-2, textareaRows)
}

func (m *Model) resizeViewport() {
	if m.viewport != nil {
		m.viewport.SetSize(m.width, m.viewportHeight())
	}
	m.textarea.SetSize(m.width-2, textareaRows)
}

func (m *Model) viewportHeight() int {
	overhead := 4 + textareaRows
	h := m.height - overhead
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) refreshViewport() {
	content := m.viewBuf + m.streamBuf
	if m.state == stateBusy && !m.hasStreamContent {
		spinChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spin := spinChars[m.spinnerFrame%len(spinChars)]
		content += spin + statusStyle.Wrap(" Thinking...\n")
	}
	if m.viewport != nil {
		atBottom := m.viewport.AtBottom()
		m.viewport.SetContent(content)
		if atBottom {
			m.viewport.ScrollToBottom()
		}
	}
}

// updateSlashHints checks whether the current textarea content starts with "/" and
// updates the slash command hint overlay. Hints are capped so they never overflow
// the viewport area.
func (m *Model) updateSlashHints() {
	text := strings.TrimSpace(m.textarea.Contents())
	if m.width == 0 || !strings.HasPrefix(text, "/") || strings.Contains(text, " ") {
		m.slashHints = ""
		return
	}

	var matching []slashCommand
	for _, sc := range slashCommands {
		if strings.HasPrefix(sc.cmd, text) {
			matching = append(matching, sc)
		}
	}

	if len(matching) == 0 {
		m.slashHints = ""
		return
	}

	// Cap visible items like Claude Code (OVERLAY_MAX_ITEMS = 5).
	const maxListItems = 5
	if text != "/" && len(matching) > maxListItems {
		matching = matching[:maxListItems]
	}

	var content string
	if text == "/" {
		content = m.renderSlashGrid(matching)
	} else {
		content = m.renderSlashList(matching)
	}

	m.slashHints = slashHintBoxStyle.Apply(content)
}

// renderSlashGrid renders slash commands in a compact multi-column grid.
func (m *Model) renderSlashGrid(cmds []slashCommand) string {
	cmdWidth := 16
	available := m.width - 4 // subtract border chars
	if available < cmdWidth {
		available = cmdWidth
	}
	cols := available / cmdWidth
	if cols < 1 {
		cols = 1
	}
	if cols > 5 {
		cols = 5
	}

	var lines []string
	var current string
	for i, sc := range cmds {
		cell := " " + slashCmdStyle.Wrap(sc.cmd)
		pad := cmdWidth - len(sc.cmd) - 1
		if pad < 1 {
			pad = 1
		}
		current += cell + strings.Repeat(" ", pad)
		if (i+1)%cols == 0 || i == len(cmds)-1 {
			lines = append(lines, current)
			current = ""
		}
	}
	return strings.Join(lines, "\n")
}

// overlaySlashHints overlays the slash command hints onto the bottom of the
// viewport output, so the hints float over the transcript area without affecting
// layout height (like Claude Code's position=absolute overlay).
func (m *Model) overlaySlashHints(vpOutput string) string {
	vpLines := strings.Split(vpOutput, "\n")
	hintLines := strings.Split(m.slashHints, "\n")

	start := len(vpLines) - len(hintLines)
	if start < 0 {
		start = 0
	}
	copy(vpLines[start:], hintLines)
	return strings.Join(vpLines, "\n")
}

// renderSlashList renders matching slash commands vertically with descriptions.
func (m *Model) renderSlashList(cmds []slashCommand) string {
	var b strings.Builder
	for i, sc := range cmds {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(" " + slashCmdStyle.Wrap(sc.cmd))
		b.WriteString("  " + statusStyle.Wrap(sc.desc))
	}
	return b.String()
}

// autocompleteSlash autocompletes the partial command to the longest common prefix
func (m *Model) autocompleteSlash(partial string) {
	var matching []string
	for _, sc := range slashCommands {
		if strings.HasPrefix(sc.cmd, partial) {
			matching = append(matching, sc.cmd)
		}
	}
	if len(matching) == 0 {
		return
	}

	common := matching[0]
	for _, cmd := range matching[1:] {
		common = commonPrefix(common, cmd)
	}
	if len(common) > len(partial) {
		m.textarea.SetContents(common)
	}
}

func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

func (m *Model) centerBox(box string) string {
	lines := strings.Split(box, "\n")
	boxWidth := 0
	for _, line := range lines {
		w := termformat.TextWidthWithANSICodes(line)
		if w > boxWidth {
			boxWidth = w
		}
	}
	leftPad := (m.width - boxWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	topPad := (m.height - len(lines)) / 2
	if topPad < 0 {
		topPad = 0
	}

	var result []string
	for i := 0; i < topPad; i++ {
		result = append(result, "")
	}
	padStr := strings.Repeat(" ", leftPad)
	for _, line := range lines {
		result = append(result, padStr+line)
	}
	return strings.Join(result, "\n")
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

func formatSessionList(metas []runtime.SessionMeta) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-40s  %-19s  %6s  %8s  %8s\n",
		"ID", "Updated", "Msgs", "In tok", "Out tok"))
	sb.WriteString(strings.Repeat("-", 90) + "\n")
	for _, m := range metas {
		ts := m.UpdatedAt.Format("2006-01-02 15:04:05")
		id := m.ID
		if len(id) > 38 {
			id = id[:35] + "..."
		}
		sb.WriteString(fmt.Sprintf("%-40s  %-19s  %6d  %8s  %8s\n",
			id, ts, m.MessageCount,
			formatNum(m.TotalInputTokens),
			formatNum(m.TotalOutputTokens)))
	}
	return sb.String()
}

func maskString(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}

func formatNum(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, c)
	}
	return string(result)
}
