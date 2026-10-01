# Griffin 的 TUI 规格原文（27 节）

> **归档说明（2026-09-25）**：这是 Griffin 交来的 TUI 规格原文，原本只在会话记录里，2026-09-24 摘出到中转根 `E:\multi_model_cowork\rb_tui_spec_griffin.md`，今天原样归档到这里（正文一个字没改）。
> 它是为 many-ai-cli 写的任务书，Council 按它做 TUI 时怎么对应、哪些做不到、哪些推后，见 `docs/15_TUI_SPEC_MAPPING.md`；运行看板（§7）的裁决与落地见 `docs/00_DECISIONS.md` #41。
> 正文里的 ✓ ◐ ○ ▶ 等字形是规格原文，Council 源码里不许照抄（符号禁令与模糊宽度，见 #41）。

---

Task: Redesign and implement the existing Many-AI CLI as a Grok Build-inspired multi-agent TUI
You are working inside an EXISTING Go codebase that has already undergone substantial development.
This is NOT a greenfield project.
Do NOT rewrite the project from scratch, do NOT migrate it to Rust, and do NOT replace existing working architecture unnecessarily.
The current project is a multi-model AI coding/agent CLI. Its long-term goal is to provide a terminal experience similar in quality and usability to Grok Build / Codex / Claude Code, while supporting flexible collaboration among many different models and agents.
Grok Build should be treated as a UX and interaction design reference, NOT as a codebase that must be copied literally.
The implementation language must remain Go.
1. First understand the existing project
Before modifying code, inspect the repository thoroughly.
Determine:

* current package/module structure
* current TUI or CLI framework
* current provider abstraction
* existing model configuration
* existing agent abstraction
* existing conversation/session handling
* streaming implementation
* tool execution system
* shell/command execution
* file editing / patch system
* context management
* current multi-model functionality
* existing commands and keyboard shortcuts
* configuration files
* logging
* persistence
* concurrency model
* any already implemented orchestration logic

Do not assume these components are missing.
Reuse working components whenever possible.
Before making architectural changes, construct a concise internal map of:

```text
existing architecture
        ↓
components that can remain unchanged
        ↓
components requiring refactoring
        ↓
new TUI components required

```

Prefer minimal, clean, incremental changes over large rewrites.
Do NOT stop after analyzing the repository. After inspection, proceed to implementation.
2. Product goal
The application should evolve into a:
Multi-model, multi-agent coding TUI with user-selectable agents, configurable collaboration strategies, shared project context, and configurable Judge agents.
The system must NOT assume that only Claude, Grok, and Codex exist.
Possible providers/models may include, but are not limited to:

```text
Claude
Grok
Codex / OpenAI
AGY
MiniMax
Gemini
Qwen
DeepSeek
OpenRouter models
local models
future providers

```

The architecture must therefore be dynamic and extensible.
No important orchestration logic should depend on hard-coded model names.
3. Core architecture
Conceptually, the application should follow this structure:

```text
                         TUI
                          │
                          ▼
                  User Configuration
                          │
                          ▼
                  Agent Registry
                          │
                          ▼
                    Orchestrator
                          │
       ┌──────────────────┼──────────────────┐
       ▼                  ▼                  ▼
   Agent A             Agent B            Agent N
       │                  │                  │
       └──────────────────┼──────────────────┘
                          ▼
                  Result Aggregator
                          │
                          ▼
                 Configurable Judge
                          │
                          ▼
                Final Answer / Action

```

Important:
Agent A, Agent B, and Agent N are dynamic runtime selections.
They are NOT fixed Claude/Grok/Codex slots.
4. Separate Provider, Model, Agent, and Role
These concepts must remain logically separate.
Provider:

```text
Anthropic
OpenAI
xAI
MiniMax
Google
AGY
OpenRouter
Local
...

```

Model:

```text
Claude Opus
Claude Sonnet
Grok
Codex
GPT
MiniMax
Gemini
Qwen
DeepSeek
...

```

Agent:
An instantiated/configured participant using a specific provider + model + role + system prompt + tool permissions.
Example:

```text
name: architect
provider: anthropic
model: claude-opus
role: architect

```

Role:

```text
Planner
Architect
Researcher
Coder
Reviewer
Critic
Debugger
Writer
Judge

```

Do not create architectural assumptions such as:

```go
claudeAgent
grokAgent
codexAgent

```

Prefer generic structures conceptually similar to:

```go
type Agent struct {
    ID       string
    Name     string
    Provider string
    Model    string
    Role     string
    Enabled  bool
}

```

Adapt this to the EXISTING codebase instead of blindly introducing duplicate types.
5. TUI design philosophy
The TUI should visually and behaviorally take inspiration from Grok Build and modern coding agents such as Codex / Claude Code.
Design principles:

* terminal-native
* clean
* compact
* professional
* keyboard-first
* minimal visual noise
* strong information hierarchy
* good streaming experience
* useful status information without becoming a monitoring dashboard
* usable for long coding sessions
* scalable from one model to many models

Avoid excessive boxes, nested borders, colorful dashboards, or permanently displaying five model columns.
The default experience should still feel like one coherent coding assistant.
Multi-agent complexity should appear only when needed.
6. Main TUI layout
Design approximately around the following hierarchy:

```text
┌──────────────────────────────────────────────────────────────────────┐
│ many-ai   project-name   branch:main   mode:ask-all                 │
│ Agents: Claude, Codex, MiniMax                Judge: Grok           │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│                     Conversation / Task Timeline                     │
│                                                                      │
│  USER                                                                │
│  Fix the nonlinear solver convergence issue.                         │
│                                                                      │
│  MULTI-AGENT RUN                                                     │
│  ┌ Claude ────────────────────────────────────────────────────────┐  │
│  │ ✓ analysis completed                                          │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌ Codex ─────────────────────────────────────────────────────────┐  │
│  │ ◐ inspecting solver.go                                        │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌ MiniMax ───────────────────────────────────────────────────────┐  │
│  │ ◐ reviewing numerical formulation                             │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  JUDGE                                                               │
│  Grok is comparing candidate solutions...                            │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│ > Ask something...                                                   │
├──────────────────────────────────────────────────────────────────────┤
│ Ctrl+P commands | Ctrl+A agents | Ctrl+J judge | Ctrl+R run mode    │
└──────────────────────────────────────────────────────────────────────┘

```

This is a conceptual layout.
Adapt it intelligently to the current TUI implementation.
7. Do not use permanent N-column model layouts
A major design requirement:
Do NOT show Claude | Grok | Codex | MiniMax | AGY permanently in separate columns.
That approach breaks quickly when the user selects many agents or uses a smaller terminal.
Instead, make the normal conversation timeline vertically oriented.
Each multi-agent invocation should appear as a grouped "run".
Example:

```text
MULTI-AGENT RUN #17

✓ Claude       18.2s
✓ Codex        24.6s
◐ Grok         streaming
○ MiniMax      queued

```

The user should be able to expand an individual result.
Example:

```text
▶ Claude      completed
▼ Codex       completed
  full Codex response here...
▶ Grok        completed
▶ MiniMax     completed

```

This keeps the normal interface clean.
8. Add an optional Compare View
For situations where the user wants to compare answers, provide an optional comparison view.
For example:

```text
Compare Responses

┌─────────────────────┬─────────────────────┐
│ Claude              │ Codex               │
│                     │                     │
│ analysis...         │ implementation...   │
│                     │                     │
├─────────────────────┼─────────────────────┤
│ Grok                │ MiniMax             │
│                     │                     │
│ critique...         │ alternative...      │
└─────────────────────┴─────────────────────┘

```

This should be a temporary view or modal, NOT the permanent main screen.
For many agents, use tabs, paging, or scrolling instead of trying to display all agents simultaneously.
9. Agent Selector
Provide an interactive Agent Selector.
Conceptually:

```text
Select agents

Search: _

Anthropic
 [x] Claude Opus
 [ ] Claude Sonnet

OpenAI
 [x] Codex
 [ ] GPT

xAI
 [x] Grok

MiniMax
 [ ] MiniMax

AGY
 [x] AGY

Local
 [ ] Qwen
 [ ] DeepSeek

--------------------------------
Selected: 4

[ Run selected ]

```

Requirements:

* multi-select
* searchable
* dynamically populated from the model/provider registry
* no hard-coded provider count
* keyboard navigation
* selected state persisted where appropriate
* gracefully handle unavailable providers/models
* show authentication/configuration problems clearly

10. Judge Selector
Judge must be independently configurable.
Example:

```text
Judge

(*) Claude Opus
( ) Grok
( ) Codex
( ) MiniMax
( ) AGY
( ) No Judge

```

The Judge may be:

```text
one model
no judge
future multi-judge strategy

```

Do not assume that the Judge is Claude or any specific provider.
The same registry used for normal agents should ideally feed the Judge selector.
11. Collaboration modes
Design the orchestration layer so the TUI can select different collaboration modes.
Initial modes should support concepts such as:

```text
direct
ask-all
parallel
sequential
review
debate
judge

```

Example semantics:

```text
direct
User → one selected agent

ask-all
User → selected agents in parallel

judge
User
 → multiple agents independently
 → collect responses
 → Judge
 → final synthesized result

review
Primary agent
 → reviewer agents
 → revised answer

debate
Agent A
 → Agent B critiques
 → Agent C critiques
 → synthesis/judge

```

Do not over-engineer every strategy immediately if the current architecture does not support them yet.
However, design the interfaces so new strategies can be added without rewriting the TUI.
12. Presets
Support reusable collaboration presets.
Examples:

```text
@fast

agents:
  - Grok
  - MiniMax

mode: parallel

```


```text
@coding

agents:
  - Claude
  - Codex
  - Grok

judge: Claude
mode: judge

```


```text
@heavy

agents:
  - Claude
  - Grok
  - Codex
  - AGY
  - MiniMax

judge: configurable
mode: judge

```

Presets should reference generic agent IDs/configuration rather than hard-coded application logic.
13. Streaming and concurrency
Multi-agent execution must feel live.
Each selected agent should independently expose states similar to:

```text
queued
starting
thinking
streaming
tool-running
waiting
completed
failed
cancelled

```

The TUI should update individual agents as events arrive.
A slow model must not block UI updates from faster models.
Use the existing Go concurrency architecture if it is already sound.
If changes are required, prefer idiomatic Go:

```text
context.Context
goroutines
channels
structured cancellation

```

Avoid uncontrolled goroutine spawning.
Cancellation must propagate cleanly.
14. Tool execution visibility
When an Agent uses tools, show concise activity.
Example:

```text
Codex
  ├─ read internal/solver.go
  ├─ search "warm_start"
  ├─ edit solver.go
  └─ run go test ./...

```

Do not flood the main conversation with raw internal events.
Tool activity should be collapsible.
Important tool operations, patches, failures, or permission requests should remain visible.
15. File modifications and safety
Multiple agents should NOT directly edit the same working tree simultaneously by default.
The first implementation should prefer:

```text
agents
   ↓
analysis / proposed patches
   ↓
judge or user selection
   ↓
one controlled writer/apply stage
   ↓
working tree

```

Preserve any existing patch approval workflow if one already exists.
Future support may use isolated git worktrees per agent, but do not introduce this unless it cleanly fits the existing system.
16. Bottom composer
The message composer should remain simple.
It should support normal prompts plus commands such as:

```text
/agents
/judge
/mode
/model
/preset
/compare
/status
/help

```

If an existing slash-command system already exists, extend it rather than replacing it.
Useful context should be visible near the composer, for example:

```text
Agents: 4
Judge: Claude
Mode: judge
Context: 61%

```

Do not overload the bottom bar.
17. Command palette
Add or improve a command palette inspired by modern coding tools.
Conceptually:

```text
> Select command

  Change agents
  Change judge
  Change collaboration mode
  Change model
  Open compare view
  View current task
  View tool activity
  View git diff
  Start new session
  Resume session
  Open settings

```

Search/filter should be supported.
Reuse existing command infrastructure when possible.
18. Keyboard-first interaction
Keyboard behavior should be consistent and discoverable.
Suggested conceptual shortcuts:

```text
Ctrl+P    command palette
Ctrl+A    agent selector
Ctrl+J    judge selector
Ctrl+R    collaboration mode
Tab       move focus
Shift+Tab previous focus
Esc       close modal / back
Enter     activate / submit
Space     toggle selection

```

Do not blindly use these exact shortcuts if they conflict with the existing application.
Preserve useful existing bindings.
Windows terminal compatibility matters.
Avoid relying unnecessarily on Unix-specific behavior.
19. Responsive terminal behavior
The TUI must gracefully handle different terminal sizes.
Prioritize roughly:

```text
>= 120 columns
full experience

80–119 columns
compact layout

< 80 columns
minimal single-pane layout

```

Do not allow content to overlap or disappear because a sidebar has a fixed width.
Optional sidebars should collapse before the conversation area becomes unusable.
20. Visual design
Use a restrained design language similar to high-quality modern coding TUIs.
Prefer:

```text
subtle borders
small number of semantic colors
clear selected/focused state
muted metadata
strong prompt/answer contrast
minimal icons
consistent spacing

```

Avoid:

```text
rainbow coloring
heavy ASCII decoration
large permanent dashboards
too many bordered boxes
large logos
unnecessary animations

```

The interface should look professional during long sessions.
Support terminals with limited color capability where practical.
21. Suggested internal TUI component structure
Do not force this exact package structure if the current project already has good organization, but conceptually identify components equivalent to:

```text
App
ConversationView
Composer
StatusBar
AgentSelector
JudgeSelector
ModeSelector
CommandPalette
MultiAgentRunView
AgentRunView
CompareView
ToolActivityView
DiffView
HelpView

```

Avoid creating a single massive TUI file.
Separate state, rendering, commands, and orchestration where practical.
22. State model
The UI should be able to represent something conceptually similar to:

```text
Application
 ├─ current session
 ├─ current workspace
 ├─ current collaboration mode
 ├─ selected agents
 ├─ selected judge
 ├─ active multi-agent run
 ├─ conversation timeline
 ├─ tool events
 ├─ modal state
 └─ focus state

```

A multi-agent run should have its own identity.
Example:

```text
Run
 ├─ run ID
 ├─ user task
 ├─ agents
 │   ├─ AgentRun A
 │   ├─ AgentRun B
 │   └─ AgentRun N
 ├─ judge
 ├─ status
 ├─ start time
 └─ final result

```

This prevents multi-agent execution state from leaking into generic UI code.
23. Preserve backward compatibility
Existing working behavior is important.
After the redesign:

* existing single-agent commands should still work unless there is a compelling reason to change them
* existing provider credentials/config should remain usable
* existing model selection should remain compatible where practical
* existing session history should not be casually broken
* existing tool execution should continue to function
* existing config files should be migrated only when necessary
* existing tests should continue passing

Do not rename or move large amounts of code purely for aesthetic reasons.
24. Implementation strategy
Perform the work incrementally.
Phase 1:
inspect existing architecture and identify reusable components.
Phase 2:
introduce or improve dynamic agent/model selection with minimal backend changes.
Phase 3:
implement the multi-agent run state required by the TUI.
Phase 4:
implement Agent Selector, Judge Selector, and collaboration mode selection.
Phase 5:
integrate concurrent streaming into the conversation timeline.
Phase 6:
implement optional Compare View and tool activity expansion.
Phase 7:
polish responsive layout, shortcuts, error states, and visual consistency.
Phase 8:
run formatting, tests, vet/static checks already used by the repository, and manually verify the major TUI workflows.
Do not unnecessarily stop between phases waiting for confirmation.
If the current code architecture makes part of this specification inappropriate, adapt the design intelligently and explain the reason in the implementation summary.
25. Critical constraints
The following constraints are mandatory:

1. Keep Go.
2. Do not rewrite the project from scratch.
3. Preserve existing working functionality.
4. Treat Grok Build as UX inspiration, not a requirement to copy Rust architecture.
5. Never hard-code the application around Claude/Grok/Codex.
6. Agent lists must be dynamic.
7. Judge must be configurable.
8. Provider, Model, Agent, and Role must remain conceptually separate.
9. Multi-agent streaming must not freeze the UI.
10. Default UI must remain clean even when many models are configured.
11. Avoid permanent one-column-per-model layouts.
12. Prefer incremental refactoring.
13. Reuse the current project's existing abstractions whenever they are already sound.
14. Keep the code maintainable and extensible for future providers.
15. Do not introduce a new dependency if the existing project already has an adequate solution.

26. Acceptance scenarios
The resulting application should support workflows like:

```text
Scenario A

User selects:
Claude
Codex
MiniMax

Mode:
ask-all

Result:
all three stream concurrently into one grouped run.

```


```text
Scenario B

User selects:
Claude
Grok
Codex
AGY
MiniMax

Judge:
Claude

Mode:
judge

Result:
five participant results are collected and sent into the Judge stage.

```


```text
Scenario C

User changes Judge:

Claude → Grok

No architectural change or recompilation should be required.

```


```text
Scenario D

A new provider is added later.

The orchestration system and TUI should discover/use its configured models through the normal registry rather than requiring new hard-coded branches throughout the application.

```


```text
Scenario E

Only one model is selected.

The application should still feel like a clean traditional coding-agent TUI rather than displaying unnecessary multi-agent UI.

```

27. Final verification
Before considering the task complete:

* build the Go project
* run existing tests
* run relevant new tests
* verify no obvious race/concurrency issue has been introduced
* verify single-agent mode
* verify multiple-agent selection
* verify concurrent streaming
* verify Judge selection
* verify mode switching
* verify cancellation
* verify terminal resizing
* verify error states when one provider fails
* verify one failed model does not destroy the whole run when other agents can still complete
* verify configuration persistence
* verify Windows compatibility where relevant

Then provide a concise implementation summary containing:

```text
What was preserved
What was changed
New architecture/components
Important files changed
New keyboard interactions
Any remaining limitations
Recommended next incremental step

```

The goal is not merely to make the interface visually similar to Grok Build.
The goal is to evolve the existing Go project into a clean, scalable multi-agent coding environment whose TUI remains simple for one model but becomes powerful when the user intentionally enables multi-model collaboration.
这个tui这样设计，之前的如果在设计的话就停掉，根据这个提示词设计Ultrathink