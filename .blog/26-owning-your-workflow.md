# WSM Standalone: Owning Your Workflow History

*Part 6 of the WSM Standalone series*

---

We've covered a lot of ground. Let's bring it together.

You have AI coding sessions scattered across multiple tools. You can't export them, can't search them, can't combine them. Your professional memory lives in vendor silos you don't control.

WSM standalone changes that. It's not another AI tool. It's the history layer that makes all your tools better.

---

## What We Built

### Post 1: History Ownership

Your coding history doesn't belong to your IDE. It belongs to you.

- Vendor-owned memory is vendor lock-in
- Your history is your professional memory
- WSM stores history in local SQLite—you own the file

### Post 2: Unified Import

One history from many tools.

- Format adapters translate Claude, Cursor, Windsurf, etc.
- Everything normalizes to a single event schema
- Merge histories from multiple tools into one timeline

### Post 3: Session Branching

Git for conversations.

- Fork from any point to explore alternatives
- Merge valuable branches back
- Discarded branches remain as institutional knowledge

### Post 4: Audit Trail

The memory you didn't know you needed.

- Every action recorded: what, when, where, who, why
- Simple queries answer impossible questions
- Automatic—you don't think about it until you need it

### Post 5: Cross-Machine Continuity

Your session, everywhere.

- Device tracking without accounts
- Sync via Git, cloud, self-hosted, or not at all
- Work flows seamlessly across laptop, desktop, VM

---

## The Complete Picture

```
┌─────────────────────────────────────────────────────────────────┐
│                      YOUR AI CODING TOOLS                       │
│   Claude Code │ Cursor │ Windsurf │ Copilot │ Gemini │ ...     │
└─────────────────────────────────────────────────────────────────┘
                         │
                         │ Import / Read / Write
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                       WSM STANDALONE                            │
│                                                                 │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐           │
│   │  Workspaces │  │  Sessions   │  │   Events    │           │
│   │  (projects) │  │  (branches) │  │  (immutable)│           │
│   └─────────────┘  └─────────────┘  └─────────────┘           │
│                                                                 │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐           │
│   │   Devices   │  │  Snapshots  │  │  Audit Log  │           │
│   │  (tracking) │  │  (caches)   │  │  (complete) │           │
│   └─────────────┘  └─────────────┘  └─────────────┘           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                         │
                         │ Your data, your choice
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                        STORAGE                                  │
│                                                                 │
│   Local SQLite  │  Git  │  Cloud Storage  │  Self-Hosted       │
│   (default)     │       │  (optional)     │  (optional)        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## What You Get

| Capability | What It Means |
|------------|---------------|
| **Ownership** | Your history in a file you control |
| **Portability** | Export, move, back up anytime |
| **Unification** | One history from many tools |
| **Exploration** | Branch, merge, preserve alternatives |
| **Accountability** | Complete audit trail |
| **Continuity** | Work across machines seamlessly |
| **Privacy** | No cloud required, no vendor dependency |
| **Query** | CLI, SQL, JSON—pick your interface |

---

## The Commands Reference

```bash
# Workspace management
wsm workspace create my-project --description "My project"
wsm workspace list
wsm workspace inspect my-project
wsm workspace archive my-project

# Session management
wsm session start my-project --name feature-x
wsm session fork --session feature-x --name exploration
wsm session merge --from exploration --into feature-x
wsm session tree feature-x

# Events
wsm events list my-project --since today
wsm events search my-project "authentication"
wsm events add my-project message.added --content "Starting work"

# Import/Export
wsm import my-project --from ~/.claude/projects/my-app/
wsm export my-project --format json --output history.json

# Devices
wsm devices list my-project

# Snapshots
wsm snapshot create my-project --name before-refactor
wsm snapshot list my-project
```

---

## Who WSM Standalone Is For

| Persona | Why They Use WSM |
|---------|------------------|
| **Individual developer** | Own their coding history, switch tools freely |
| **Team lead** | Onboarding, decision archaeology, incident review |
| **Compliance officer** | Audit trails, access tracking, evidence export |
| **Privacy-conscious** | No cloud dependency, local-first data |
| **Tool explorer** | Try new AI tools without losing context |

---

## Who WSM Standalone Is NOT For

| If You Want | WSM Doesn't |
|-------------|-------------|
| Agent management | WSM doesn't manage AI agent profiles |
| Multi-agent orchestration | See the Squad Topologies series |
| Cloud-native everything | WSM is local-first by design |
| Zero setup | WSM requires initial import/configuration |

For those use cases, consider the full Hop ecosystem with APS integration.

---

## Getting Started

### 1. Install

```bash
# Clone and build
git clone https://github.com/IdeaCraftersLabs/wsm.git
cd wsm
make build

# Or download release binary
# (when available)
```

### 2. Create a Workspace

```bash
wsm workspace create my-project --description "My coding project"
```

### 3. Import Existing History

```bash
# Import from your primary AI tool
wsm import my-project --from ~/.claude/projects/my-project/
```

### 4. Start Using

```bash
# Record events as you work
wsm events add my-project message.added --content "Working on auth"

# Query when you need answers
wsm events search my-project "why"
```

### 5. Make It Automatic

Configure your AI tools to read/write WSM directly:

```bash
# Claude Code integration (future)
export CLAUDE_HISTORY_BACKEND=wsm
export WSM_WORKSPACE=my-project
```

---

## The Philosophy

WSM standalone is built on a few core beliefs:

1. **Your data belongs to you** — Not to Anthropic, not to Cursor, not to anyone else
2. **History is valuable** — More valuable than code, because code is the result
3. **Tools should serve you** — Not the other way around
4. **Privacy is a right** — Not a premium feature
5. **Simplicity wins** — Local SQLite, open formats, no vendor lock-in

---

## The Series Recap

| Post | Core Idea |
|------|-----------|
| 1 | Your coding history doesn't belong to your IDE |
| 2 | Import everything into one unified history |
| 3 | Branch conversations like you branch code |
| 4 | The audit trail you didn't know you needed |
| 5 | Cross-machine continuity without the cloud |
| 6 | Own your workflow history |

---

## What's Next

This series covered WSM standalone—the history layer for individual developers.

For multi-agent orchestration, see the **Squad Topologies** series.

For cross-IDE session collaboration, see the **Cross-IDE Sessions** series.

For full ecosystem integration with APS and Hop, see the integration documentation.

---

Your coding history is your professional memory. Treat it like the asset it is.

Start owning it today.

---

*This post is part of the WSM Standalone series. [← Back to Part 5](25-cross-machine-continuity.md) | [Back to Part 1](20-your-history-doesnt-belong-to-your-ide.md)*
