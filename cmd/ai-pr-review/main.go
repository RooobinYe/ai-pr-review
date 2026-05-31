package main

import (
	"ai-pr-review/internal/auth"
	"ai-pr-review/internal/commands"
	"ai-pr-review/internal/compat"
	"ai-pr-review/internal/permissions"
	"ai-pr-review/internal/pr"
	"ai-pr-review/internal/prompt"
	"ai-pr-review/internal/review"
	"ai-pr-review/internal/runtime"
	"ai-pr-review/internal/tui"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	qtui "ai-pr-review/internal/q/tui"
)

// These package-level variables can be injected at build time via ldflags to
// produce a zero-config binary:
//
//	go build -ldflags "\
//	  -X 'main.DefaultAPIKey=sk-xxx' \
//	  -X 'main.DefaultBaseURL=https://api.example.com' \
//	  -X 'main.DefaultModel=claude-sonnet-4-20250514' \
//	  -X 'main.Version=1.0.0'" \
//	  ./cmd/ai-pr-review
//
// Each serves as the lowest-priority fallback so that env vars, CLI flags, and
// stored credentials all take precedence over the embedded value.
var (
	DefaultAPIKey  string
	DefaultBaseURL string
	DefaultModel   string
)

// Version is injected at build time via ldflags. It is embedded in the JSON
// output so that consumers can identify which version of ai-pr-review produced
// the review.
var Version string

func main() {
	// Route diagnostic subcommands before flag parsing.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "dump-manifests":
			compat.RunDumpManifests(os.Args[2:])
			return
		case "bootstrap-plan":
			compat.RunBootstrapPlan(os.Args[2:])
			return
		case "print-system-prompt":
			compat.RunPrintSystemPrompt(os.Args[2:])
			return
		case "resume-session":
			compat.RunResumeSession(os.Args[2:])
			return
		}
	}

	promptFlag := flag.String("prompt", "", "Run a single prompt and exit")
	prFlag := flag.String("pr", "", "GitHub PR URL to review (e.g. https://github.com/owner/repo/pull/123)")
	formatFlag := flag.String("format", "", "Output format: markdown or json (requires --pr)")
	quietFlag := flag.Bool("quiet", false, "Suppress all stderr diagnostic output (use in CI/CD)")
	outputFlag := flag.String("output", "", "Write JSON result to file")
	failOnFlag := flag.String("fail-on", "", "Exit non-zero if must_fix contains issues at this severity or above (critical|high|medium|low)")
	modelFlag := flag.String("model", "", "Override the model to use")
	apiKeyFlag := flag.String("api-key", "", "API key for the AI provider (overrides embedded default)")
	baseURLFlag := flag.String("base-url", "", "Base URL for the AI provider API (overrides embedded default)")
	replFlag := flag.Bool("repl", false, "Run in interactive REPL mode (default when no --prompt)")
	sessionFlag := flag.String("session", "", "Session ID to load")
	sessionDirFlag := flag.String("session-dir", "", "Directory to store sessions")
	permModeFlag := flag.String("permission-mode", "default", "Permission mode: default, accept-edits, bypass, plan")
	_ = replFlag

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ai-pr-review [subcommand] [options]\n\n")
		fmt.Fprintf(os.Stderr, "Subcommands:\n")
		fmt.Fprintf(os.Stderr, "  dump-manifests [--src <dir>] [--json]   List tools, slash commands, and source manifest\n")
		fmt.Fprintf(os.Stderr, "  bootstrap-plan [--json]                 Print the ordered startup phase plan\n")
		fmt.Fprintf(os.Stderr, "  print-system-prompt [--cwd] [--date]    Render the full system prompt\n")
		fmt.Fprintf(os.Stderr, "  resume-session <file> [commands...]     Replay a saved session file\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nPR Review:\n")
		fmt.Fprintf(os.Stderr, "  ai-pr-review --pr https://github.com/owner/repo/pull/123\n")
		fmt.Fprintf(os.Stderr, "  ai-pr-review --pr https://github.com/owner/repo/pull/123 --format markdown\n")
		fmt.Fprintf(os.Stderr, "  ai-pr-review --pr https://github.com/owner/repo/pull/123 使用中文回答\n")
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  GITHUB_TOKEN             GitHub personal access token (recommended for --pr)\n")
		fmt.Fprintf(os.Stderr, "  ANTHROPIC_API_KEY        Anthropic API key (takes precedence over stored credentials)\n")
		fmt.Fprintf(os.Stderr, "  OPENAI_API_KEY           OpenAI API key (takes precedence over stored credentials)\n")
		fmt.Fprintf(os.Stderr, "  ANTHROPIC_MODEL          Model to use (default: %s)\n", runtime.DefaultModel)
		fmt.Fprintf(os.Stderr, "  ANTHROPIC_BASE_URL       Base URL for the Anthropic API\n")
		fmt.Fprintf(os.Stderr, "  CLAUDE_CODE_USE_BEDROCK  Set to 1 to use AWS Bedrock (env-var fallback)\n")
		fmt.Fprintf(os.Stderr, "  CLAUDE_CODE_USE_VERTEX   Set to 1 to use Google Vertex AI (env-var fallback)\n")
		fmt.Fprintf(os.Stderr, "  CLAUDE_CODE_USE_FOUNDRY  Set to 1 to use Azure AI Foundry (env-var fallback)\n")
		fmt.Fprintf(os.Stderr, "\nCredential precedence (highest to lowest):\n")
		fmt.Fprintf(os.Stderr, "  1. Environment variable (ANTHROPIC_API_KEY / OPENAI_API_KEY)\n")
		fmt.Fprintf(os.Stderr, "  2. --api-key CLI flag\n")
		fmt.Fprintf(os.Stderr, "  3. Embedded at build time (ldflags -X main.DefaultAPIKey=...)\n")
		fmt.Fprintf(os.Stderr, "  4. Stored credentials (~/.ai-pr-review/credentials.json from /login)\n")
		fmt.Fprintf(os.Stderr, "  5. Settings file (.ai-pr-review/settings.json / settings.local.json)\n")
	}

	flag.Parse()

	// Extra args after flags become natural-language instructions for the review.
	extraArgs := strings.Join(flag.Args(), " ")

	// prInitialPrompt carries the PR analysis prompt into the TUI when --pr is
	// used without --format.
	var prInitialPrompt string

	// When --format is active, route all diagnostic output to stderr so stdout
	// stays clean for the final JSON/markdown output. In --quiet mode, suppress
	// diagnostic stderr as well via errOut.
	var errOut io.Writer = os.Stderr
	if *quietFlag || os.Getenv("CI") != "" {
		errOut = io.Discard
	}

	// When --format is active, use prOut for PR metadata (stderr when format mode,
	// stdout otherwise). This keeps stdout clean for the final formatted output.
	prOut := io.Writer(os.Stdout)
	if *formatFlag != "" {
		prOut = errOut // route PR progress to stderr (suppressible by --quiet)
	}

	// If --pr is specified, validate the URL, fetch PR data, and print summary.
	var prInfo *pr.PRInfo
	if *prFlag != "" {
		info, err := pr.ParsePRURL(*prFlag)
		if err != nil {
			fmt.Fprintf(errOut, "Error: invalid PR URL %q: %v\n", *prFlag, err)
			fmt.Fprintln(errOut, "Expected format: https://github.com/owner/repo/pull/number")
			os.Exit(1)
		}
		prInfo = info

		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			fmt.Fprintln(errOut, "Warning: GITHUB_TOKEN environment variable is not set.")
			fmt.Fprintln(errOut, "         Public repos will work with strict rate limiting (60 req/hour).")
			fmt.Fprintln(errOut, "         For private repos or higher limits, set: export GITHUB_TOKEN=<your-github-token>")
		}

		fmt.Fprintf(prOut, "PR Review: %s/%s #%d\n", prInfo.Owner, prInfo.Repo, prInfo.PullNumber)
		if *formatFlag == "" {
			fmt.Println()
		}

		client := pr.NewGitHubClient(token)
		ctx := context.Background()

		fmt.Fprintf(prOut, "Fetching PR details...\n")
		prData, fetchErr := client.FetchPR(ctx, prInfo.Owner, prInfo.Repo, prInfo.PullNumber)
		if fetchErr != nil {
			fmt.Fprintf(errOut, "Error: failed to fetch PR data: %v\n", fetchErr)
			os.Exit(1)
		}

		// Print PR summary to prOut (stderr in format mode, stdout otherwise).
		fmt.Fprintf(prOut, "  Title:   %s\n", prData.Details.Title)
		fmt.Fprintf(prOut, "  Author:  %s\n", prData.Details.Author)
		fmt.Fprintf(prOut, "  State:   %s\n", prData.Details.State)
		fmt.Fprintf(prOut, "  Branch:  %s ← %s\n", prData.Details.BaseBranch, prData.Details.HeadBranch)
		fmt.Fprintf(prOut, "  Commits: %s (base) ↔ %s (head)\n", shortSHA(prData.Details.BaseSHA), shortSHA(prData.Details.HeadSHA))
		fmt.Fprintf(prOut, "  URL:     %s\n", prData.Details.URL)

		// Clone the PR's head repository so the AI can explore full file context
		// using read_file, grep, glob, and bash tools.
		cloneBaseDir, cloneDirErr := pr.DefaultCloneBaseDir()
		if cloneDirErr != nil {
			fmt.Fprintf(errOut, "Warning: %v\n", cloneDirErr)
		}
		cloneMgr := pr.NewCloneManager(cloneBaseDir)
		cloneDisplay := cloneBaseDir
		if cloneDisplay == "" {
			cloneDisplay = "~/.ai-pr-review/pr"
		}
		fmt.Fprintf(prOut, "\nCloning repository into %s/%s-%s-%d/ ...\n",
			cloneDisplay, prInfo.Owner, prInfo.Repo, prInfo.PullNumber)
		repoPath, cloneErr := cloneMgr.ClonePRRepo(ctx,
			prInfo.Owner, prInfo.Repo, prInfo.PullNumber,
			prData.Details.HeadBranch, prData.Details.CloneURL, token)
		if cloneErr != nil {
			fmt.Fprintf(errOut, "Warning: could not clone repository: %v\n", cloneErr)
			fmt.Fprintln(errOut, "         Review will proceed with diff-only context.")
		} else {
			fmt.Fprintf(prOut, "  Cloned to: %s\n", repoPath)
			if err := os.Chdir(repoPath); err != nil {
				fmt.Fprintf(errOut, "Warning: could not change to repo directory: %v\n", err)
			}
		}
		if *formatFlag == "" {
			fmt.Println()
		}

		// --format: run review pipeline and exit immediately.
		if *formatFlag != "" {
			runReviewPipeline(prData, *formatFlag, *modelFlag, *apiKeyFlag, *baseURLFlag, extraArgs, *quietFlag, *outputFlag, *failOnFlag)
			return
		}

		// No --format: build the initial PR analysis prompt for the TUI.
		prInitialPrompt = buildPRPrompt(prData, extraArgs)
		// Fall through to the TUI path below.
	}

	cfg := runtime.LoadConfig()

	if *modelFlag != "" {
		cfg.Model = *modelFlag
	} else if cfg.Model == runtime.DefaultModel && DefaultModel != "" {
		// Use the ldflags-injected model as a fallback over the hardcoded default.
		cfg.Model = DefaultModel
	}
	if *sessionDirFlag != "" {
		cfg.SessionDir = *sessionDirFlag
	}

	// Resolve the effective API key: CLI flag > ldflags embedded default.
	effectiveKey := *apiKeyFlag
	if effectiveKey == "" {
		effectiveKey = DefaultAPIKey
	}

	// Resolve the effective base URL: CLI flag > ldflags embedded default.
	// cfg.BaseURL was already populated from settings files + ANTHROPIC_BASE_URL
	// env var by LoadConfig, so only override when the user explicitly asks.
	if *baseURLFlag != "" {
		cfg.BaseURL = *baseURLFlag
	} else if cfg.BaseURL == "" && DefaultBaseURL != "" {
		cfg.BaseURL = DefaultBaseURL
	}

	// Resolve credentials using the multi-provider credential store.
	// Env vars take precedence (ANTHROPIC_API_KEY, OPENAI_API_KEY).
	// Falls back gracefully so the TUI can start and prompt the user to /login.
	provider, token, authMethod, credErr := auth.ResolveCredentials()
	if credErr == nil {
		cfg.ProviderName = provider
		cfg.AuthMethod = authMethod
		if authMethod == "oauth" {
			cfg.OAuthToken = token
		} else {
			cfg.APIKey = token
		}
	} else if effectiveKey != "" {
		// Use the API key from CLI flag or ldflags as fallback.
		cfg.APIKey = effectiveKey
		cfg.AuthMethod = "api_key"
		credErr = nil
	} else {
		// No credentials found — start with NoAuthClient so the TUI still opens.
		// The user can run /login inside the TUI.
		fmt.Fprintf(os.Stderr, "Note: no credentials found (%v).\n", credErr)
		fmt.Fprintln(os.Stderr, "      Use /login in the TUI to authenticate.")
	}

	// Create the provider client (or a no-auth placeholder).
	realClient, clientErr := runtime.NewProviderClient(cfg)
	if clientErr != nil {
		fmt.Fprintf(os.Stderr, "Note: could not create %s client: %v\n", cfg.ProviderName, clientErr)
		fmt.Fprintln(os.Stderr, "      Use /login in the TUI to authenticate.")
		realClient = runtime.NewNoAuthClient()
	}

	loop := runtime.NewConversationLoop(cfg, realClient)

	// When in PR review mode, inject the review-specific system prompt
	// that adds anti-hallucination rules, failure modes, and review criteria.
	if prInitialPrompt != "" {
		loop.SystemPromptOverride = prompt.AgenticReviewSystemPrompt()
	}

	// Wire up the permission manager (Phase 11).
	// CLI --permission-mode flag overrides the config-file value when set to a
	// non-default value. cfg.PermissionMode comes from the layered settings files.
	resolvedPermMode := cfg.PermissionMode
	if *permModeFlag != "default" {
		resolvedPermMode = *permModeFlag
	}
	permMode, err := permissions.ParsePermissionMode(resolvedPermMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v; using default mode\n", err)
		permMode = permissions.ModeDefault
	}
	cfg.PermissionMode = permMode.String() // normalise back into Config

	ruleset, rErr := permissions.LoadRuleset(".ai-pr-review/settings.json")
	if rErr != nil {
		ruleset = &permissions.Ruleset{}
	}
	// Merge allowedTools/blockedTools from layered config into the ruleset.
	if len(cfg.AllowedTools) > 0 || len(cfg.BlockedTools) > 0 {
		extra := permissions.RulesetFromLists(cfg.AllowedTools, cfg.BlockedTools)
		ruleset.Rules = append(ruleset.Rules, extra.Rules...)
	}
	loop.PermManager = permissions.NewManager(permMode, ruleset)

	// Connect to MCP servers defined in config (non-fatal errors printed inside).
	loop.InitMCPFromConfig(context.Background())

	if *sessionFlag != "" {
		sess, err := runtime.LoadSession(cfg.SessionDir, *sessionFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load session %s: %v\n", *sessionFlag, err)
		} else {
			loop.Session = sess
			fmt.Printf("Loaded session: %s\n", sess.ID)
		}
	}

	// Single prompt (non-interactive) mode — no TUI, plain stdout streaming.
	if *promptFlag != "" {
		if credErr != nil && cfg.APIKey == "" && cfg.OAuthToken == "" {
			fmt.Fprintln(os.Stderr, "Error: cannot use --prompt without valid credentials.")
			fmt.Fprintln(os.Stderr, "  Pass --api-key <key>")
			fmt.Fprintln(os.Stderr, "  Set ANTHROPIC_API_KEY or OPENAI_API_KEY")
			fmt.Fprintln(os.Stderr, "  Or run the TUI and use /login to authenticate.")
			os.Exit(1)
		}
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stdout, "\nInterrupted. Saving session...")
			saveSessionSilent(cfg.SessionDir, loop)
			os.Exit(0)
		}()

		ctx := context.Background()
		if err := loop.SendMessage(ctx, *promptFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		saveSessionSilent(cfg.SessionDir, loop)
		return
	}

	// Interactive TUI mode.
	runTUI(cfg, loop, prInitialPrompt)
}

// buildPRPrompt constructs the initial analysis prompt for --pr TUI mode.
// extraArgs contains optional user instructions appended after the PR context
// (e.g. "使用中文回答", "focus on security issues").
func buildPRPrompt(data *pr.PRData, extraArgs string) string {
	// Use the centralised prompt builder from the prompt package.
	// It provides XML-structured prompts with exploration checklists
	// and anti-hallucination rules.
	return prompt.AgenticUserPrompt(data, extraArgs)
}

func sumAdditions(files []pr.ChangedFile) int {
	total := 0
	for _, f := range files {
		total += f.Additions
	}
	return total
}

func sumDeletions(files []pr.ChangedFile) int {
	total := 0
	for _, f := range files {
		total += f.Deletions
	}
	return total
}

func formatCatCounts(counts map[review.FileCategory]int) string {
	order := []review.FileCategory{
		review.FileCategoryCode,
		review.FileCategoryTest,
		review.FileCategoryConfig,
		review.FileCategoryDoc,
		review.FileCategoryOther,
	}
	var parts []string
	for _, cat := range order {
		if n := counts[cat]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, cat.String()))
		}
	}
	return strings.Join(parts, ", ")
}

// runReviewPipeline runs the full AI review with repo context exploration and outputs
// formatted results. For markdown format it runs a single agentic loop (explore +
// review). For JSON format it uses a two-phase approach:
//   - Phase 1: explore the repository with tools (same as markdown)
//   - Phase 2: produce structured JSON with must_fix and points_of_interest
func runReviewPipeline(prData *pr.PRData, format, modelFlag, apiKeyFlag, baseURLFlag, extraArgs string, quiet bool, outputFile, failOn string) {
	// Resolve the effective API key: CLI flag > ldflags embedded default.
	effectiveKey := apiKeyFlag
	if effectiveKey == "" {
		effectiveKey = DefaultAPIKey
	}

	// Determine stderr writer. In --quiet mode or CI environments, suppress all
	// diagnostic output so that stdout contains only the JSON result.
	errOut := io.Writer(os.Stderr)
	if quiet || os.Getenv("CI") != "" {
		errOut = io.Discard
	}

	format = strings.ToLower(format)
	if format != "markdown" && format != "json" {
		emitErrorJSON("USAGE_ERROR", fmt.Sprintf("unsupported format %q, use \"markdown\" or \"json\"", format), 4)
		return
	}

	// Use the exact same config + credential resolution as the TUI path so that
	// BaseURL, provider selection, and all credential sources are consistent.
	cfg := runtime.LoadConfig()

	if modelFlag != "" {
		cfg.Model = modelFlag
	} else if cfg.Model == runtime.DefaultModel && DefaultModel != "" {
		cfg.Model = DefaultModel
	}

	// Base URL: CLI flag > ldflags embedded default.
	if baseURLFlag != "" {
		cfg.BaseURL = baseURLFlag
	} else if cfg.BaseURL == "" && DefaultBaseURL != "" {
		cfg.BaseURL = DefaultBaseURL
	}

	providerName, token, authMethod, credErr := auth.ResolveCredentials()
	if credErr == nil {
		cfg.ProviderName = providerName
		cfg.AuthMethod = authMethod
		if authMethod == "oauth" {
			cfg.OAuthToken = token
		} else {
			cfg.APIKey = token
		}
	} else if effectiveKey != "" {
		cfg.APIKey = effectiveKey
		cfg.AuthMethod = "api_key"
	}

	if cfg.APIKey == "" && cfg.OAuthToken == "" {
		emitErrorJSON("NO_CREDENTIALS", "no credentials found — pass --api-key, set ANTHROPIC_API_KEY, or run /login", 1)
		return
	}

	// Create the provider client through the same factory used by the TUI.
	providerClient, clientErr := runtime.NewProviderClient(cfg)
	if clientErr != nil {
		emitErrorJSON("CLIENT_INIT_FAILED", fmt.Sprintf("could not create %s client: %v", cfg.ProviderName, clientErr), 2)
		return
	}

	// Use ConversationLoop to give the AI access to tools (read_file, grep,
	// glob, bash) so it can explore the full repo context.
	loop := runtime.NewConversationLoop(cfg, providerClient)

	// Bypass permission prompts — there is no user to answer them in --format mode.
	loop.PermManager = permissions.NewManager(permissions.ModeBypassPermissions, &permissions.Ruleset{})

	// Build the exploration prompt (same as TUI mode).
	explorePrompt := buildPRPrompt(prData, extraArgs)

	var output string
	model := cfg.Model
	var warnings []string

	if format == "json" {
		// ---- Two-phase JSON review ----

		// Phase 1: explore the repository with tools.
		fmt.Fprintf(errOut, "Phase 1/2: Exploring repository context...\n")
		if _, err := runAgenticReview(loop, explorePrompt); err != nil {
			emitErrorJSON("AI_EXPLORATION_FAILED", fmt.Sprintf("exploration failed: %v", err), 2)
			return
		}

		// Phase 2: ask the AI to produce structured JSON based on everything it learned.
		fmt.Fprintf(errOut, "Phase 2/2: Generating structured JSON review...\n")
		structuredPrompt := buildStructuredJSONPrompt()
		rawResponse, err := runAgenticReview(loop, structuredPrompt)
		if err != nil {
			emitErrorJSON("AI_REVIEW_FAILED", fmt.Sprintf("structured review failed: %v", err), 2)
			return
		}

		output = formatReviewJSON(rawResponse, prData, model, errOut, warnings)

		// --fail-on: check must_fix severity and exit non-zero if threshold met.
		if failOn != "" {
			if exitCode := checkFailOn(output, failOn); exitCode != 0 {
				fmt.Println(output)
				os.Exit(exitCode)
			}
		}
	} else {
		// ---- Single-phase markdown review ----
		fmt.Fprintf(errOut, "Running AI review with full repo context...\n")
		reviewText, err := runAgenticReview(loop, explorePrompt)
		if err != nil {
			emitErrorJSON("AI_REVIEW_FAILED", fmt.Sprintf("review failed: %v", err), 2)
			return
		}
		output = reviewText
	}

	// Write to file if --output is specified.
	if outputFile != "" {
		if err := os.WriteFile(outputFile, []byte(output), 0644); err != nil {
			emitErrorJSON("FILE_WRITE_FAILED", fmt.Sprintf("could not write output file: %v", err), 4)
			return
		}
	}

	fmt.Println(output)
}

// emitErrorJSON writes a structured error as JSON to stdout and exits with the
// given code. This ensures that even on failure, CI pipelines receive parseable
// JSON on stdout — never an empty response.
func emitErrorJSON(code, message string, exitCode int) {
	result := review.ErrorOutput{
		SchemaVersion: review.CurrentSchemaVersion,
		ToolVersion:   Version,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Error: review.ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
	os.Exit(exitCode)
}

// buildStructuredJSONPrompt returns the Phase 2 prompt that asks the AI to produce
// a structured JSON review after it has already explored the repository.
func buildStructuredJSONPrompt() string {
	return `You have already explored the repository and understand the PR changes.
Now produce a final structured code review as JSON.

Return ONLY valid JSON (no markdown fences, no commentary). Use this exact schema:

{
  "summary": "3-5 sentence overview of what this PR changes and why",
  "limitations": ["list of analysis limitations you encountered"],
  "must_fix": [
    {
      "file": "path/to/file.go",
      "line": 0,
      "severity": "critical|high|medium",
      "confidence": "high|medium",
      "category": "security|nil-pointer|error-handling|performance|logic|concurrency|style|other",
      "title": "short risk title (max 10 words)",
      "evidence": "specific code snippet or diff excerpt that demonstrates the issue",
      "description": "detailed explanation of the issue (max 3 sentences)",
      "suggestion": "specific, actionable fix suggestion",
      "uncertainty": "required if confidence is medium; explain what additional context would increase confidence"
    }
  ],
  "points_of_interest": [
    {
      "file": "path/to/file.go",
      "line": 0,
      "severity": "low|info",
      "confidence": "low",
      "category": "security|nil-pointer|error-handling|performance|logic|concurrency|style|other",
      "title": "short title",
      "evidence": "code reference if available, or empty string",
      "description": "detailed explanation",
      "suggestion": "actionable suggestion",
      "uncertainty": "REQUIRED for all points_of_interest; explain why the finding is uncertain"
    }
  ]
}

Classification rules:
- must_fix: HIGH or MEDIUM confidence issues that SHOULD be addressed before merging.
  severity must be critical, high, or medium.
- points_of_interest: LOW confidence findings, speculative issues, style nits, or
  nice-to-have improvements that do not block merge. severity must be low or info.
- severity reflects impact: critical = security hole or data loss, high = likely bug,
  medium = potential issue, low = minor, info = observation.
- confidence reflects certainty: high = clear-cut issue, medium = likely issue,
  low = speculative (low-confidence items ALWAYS go to points_of_interest).

Field requirements:
- "evidence" is REQUIRED for all must_fix entries (include the exact code or diff).
- "uncertainty" is REQUIRED for all points_of_interest entries.
- "uncertainty" is REQUIRED for must_fix entries with confidence="medium".

Guidelines:
- Only include real, concrete findings. Do not fabricate issues.
- Be specific — reference exact file paths and line numbers.
- For each finding provide an actionable suggestion.
- If you found no issues in a category, return an empty array.
- Maximum 15 findings total across both arrays.
- Return ONLY the JSON object, no markdown fences or surrounding text.`
}

// runAgenticReview sends the prompt through the conversation loop and collects all
// text output. It runs the loop in a background goroutine and drains events from the
// channel, collecting text deltas and auto-answering any permission/user prompts
// (since --format mode is non-interactive).
func runAgenticReview(loop *runtime.ConversationLoop, prompt string) (string, error) {
	ctx := context.Background()
	events := make(chan runtime.TurnEvent, 100)

	var fullText strings.Builder
	errCh := make(chan error, 1)

	go func() {
		errCh <- loop.SendMessageStreaming(ctx, prompt, events)
		close(events)
	}()

	for event := range events {
		switch event.Type {
		case runtime.TurnEventTextDelta:
			fullText.WriteString(event.Text)
		case runtime.TurnEventError:
			return "", event.Err
		case runtime.TurnEventPermissionAsk:
			// Auto-allow in non-interactive mode.
			event.PermReply <- runtime.PermDecisionAllowOnce
		case runtime.TurnEventAskUser:
			// Auto-reply in non-interactive mode.
			event.AskUserReply <- "N/A (non-interactive mode)"
		}
		// TurnEventToolStart, TurnEventToolDone, TurnEventUsage, TurnEventDone
		// are informational — no action needed.
	}

	if err := <-errCh; err != nil {
		return "", err
	}

	return fullText.String(), nil
}

// formatReviewJSON parses the AI's JSON response and produces a typed ReviewOutput
// with PR metadata, CI-friendly headers, and file change summaries.
// If the AI response is not valid JSON, it falls back to wrapping the raw text.
func formatReviewJSON(rawResponse string, prData *pr.PRData, model string, errOut io.Writer, warnings []string) string {
	cleaned := extractJSON(rawResponse)

	var aiResult struct {
		Summary          string        `json:"summary"`
		Limitations      []string      `json:"limitations"`
		MustFix          []review.Risk `json:"must_fix"`
		PointsOfInterest []review.Risk `json:"points_of_interest"`
	}
	if err := json.Unmarshal([]byte(cleaned), &aiResult); err != nil {
		fmt.Fprintf(errOut, "Warning: could not parse AI JSON response (%v).\n", err)
		fmt.Fprintln(errOut, "         Outputting raw response wrapped in JSON.")
		return fallbackJSONWrap(rawResponse, prData, model, append(warnings,
			fmt.Sprintf("AI JSON parse failed: %v", err)))
	}

	// Surface AI-reported limitations as warnings.
	for _, lim := range aiResult.Limitations {
		warnings = append(warnings, "AI limitation: "+lim)
	}

	result := review.ReviewOutput{
		SchemaVersion:    review.CurrentSchemaVersion,
		ToolVersion:      Version,
		Model:            model,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		PRInfo:           prData.Info,
		Title:            prData.Details.Title,
		Author:           prData.Details.Author,
		BaseBranch:       prData.Details.BaseBranch,
		HeadBranch:       prData.Details.HeadBranch,
		FileChanges:      buildFileChanges(prData),
		Summary:          aiResult.Summary,
		MustFix:          aiResult.MustFix,
		PointsOfInterest: aiResult.PointsOfInterest,
		Warnings:         warnings,
	}

	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"schema_version":%d,"error":{"code":"JSON_MARSHAL_FAILED","message":%q}}`,
			review.CurrentSchemaVersion, err.Error())
	}
	return string(b)
}

// fallbackJSONWrap wraps raw text in a typed ReviewOutput when the AI did not
// return valid JSON. This ensures the output is always parseable by jq.
func fallbackJSONWrap(rawText string, prData *pr.PRData, model string, warnings []string) string {
	result := review.ReviewOutput{
		SchemaVersion:    review.CurrentSchemaVersion,
		ToolVersion:      Version,
		Model:            model,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		PRInfo:           prData.Info,
		Title:            prData.Details.Title,
		Author:           prData.Details.Author,
		BaseBranch:       prData.Details.BaseBranch,
		HeadBranch:       prData.Details.HeadBranch,
		FileChanges:      buildFileChanges(prData),
		Summary:          "",
		MustFix:          []review.Risk{},
		PointsOfInterest: []review.Risk{},
		Warnings:         append(warnings, "AI did not return valid JSON — raw review preserved in raw_review"),
		RawReview:        rawText,
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	return string(b)
}

// buildFileChanges builds FileSummary entries from the PR's changed files list.
func buildFileChanges(prData *pr.PRData) []review.FileSummary {
	changes := make([]review.FileSummary, 0, len(prData.Files))
	for _, f := range prData.Files {
		changes = append(changes, review.FileSummary{
			Filename:  f.Filename,
			Category:  review.ClassifyFile(f.Filename),
			Summary:   "",
			Additions: f.Additions,
			Deletions: f.Deletions,
		})
	}
	return changes
}

// checkFailOn parses the JSON output and checks whether any must_fix entry meets
// or exceeds the given severity threshold. Returns exit code 5 if the threshold
// is met, 0 otherwise.
func checkFailOn(output, failOn string) int {
	threshold := review.ParseSeverity(failOn)
	if threshold == review.RiskSeverityInfo && failOn != "info" {
		return 0
	}

	var result review.ReviewOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return 0
	}

	for _, r := range result.MustFix {
		if r.Severity <= threshold {
			return 5
		}
	}
	return 0
}

// extractJSON strips markdown code fences and leading/trailing whitespace from a
// string that is expected to contain JSON.
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	// Strip opening fence: ```json or just ```
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
	}
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// runTUI starts the Bubble Tea TUI for interactive use.
func runTUI(cfg *runtime.Config, loop *runtime.ConversationLoop, initialPrompt string) {
	// Register slash commands (available for future non-TUI REPL mode).
	registry := commands.NewRegistry()
	commands.RegisterAuthCommands(registry)
	commands.RegisterMCPCommand(registry)
	_ = registry

	// Save session on SIGTERM (Ctrl+C is handled by Bubble Tea itself).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		<-sigCh
		saveSessionSilent(cfg.SessionDir, loop)
		os.Exit(0)
	}()

	model := tui.NewModel(cfg, loop, initialPrompt)
	if err := qtui.RunTUI(model, qtui.Options{
		Framerate:   60,
		EnableMouse: true,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}

	// Save session after the TUI exits (covers Ctrl+C via tea.Quit).
	saveSessionSilent(cfg.SessionDir, loop)
}

// saveSessionSilent saves the session, printing only to stderr on failure.
func saveSessionSilent(dir string, loop *runtime.ConversationLoop) {
	if err := runtime.SaveSession(dir, loop.Session); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save session: %v\n", err)
	}
}

// shortSHA returns the first 7 characters of a commit SHA.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// statusTag returns a colored tag for a file change status.
func statusTag(status string) string {
	switch status {
	case "added":
		return "[+]"
	case "removed":
		return "[-]"
	case "renamed":
		return "[→]"
	case "modified":
		return "[~]"
	default:
		return "[ ]"
	}
}

// wrapLines wraps text to a maximum width, preserving existing newlines.
func wrapLines(text string, width int) []string {
	if text == "" {
		return nil
	}
	var lines []string
	for _, paragraph := range splitLines(text) {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		runes := []rune(paragraph)
		for len(runes) > width {
			// Find a good break point.
			br := width
			for br > 0 && runes[br] != ' ' {
				br--
			}
			if br == 0 {
				br = width // force break
			}
			lines = append(lines, string(runes[:br]))
			if runes[br] == ' ' {
				br++
			}
			runes = runes[br:]
		}
		if len(runes) > 0 {
			lines = append(lines, string(runes))
		}
	}
	return lines
}

// splitLines splits text by newlines, like strings.Split but without the allocation pattern.
func splitLines(text string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			lines = append(lines, text[start:i])
			start = i + 1
		}
	}
	lines = append(lines, text[start:])
	return lines
}
