# Session Forking: Branch, Explore, Merge

*Part 4 of the Cross-IDE Sessions series*

---

Debugging isn't linear. Neither is design. You explore multiple paths, hit dead ends, backtrack, try again.

Your session history should reflect that reality. Not flatten it into a single timeline that never existed.

---

## The Problem: Linear History for Non-Linear Work

Most chat logs are linear. Message after message, in sequence. But that's not how you actually work:

```
You: "Let's try approach A"
AI: "Here's approach A..."
[20 minutes later]
You: "That didn't work. Let me try approach B"
AI: "Here's approach B..."
[15 minutes later]  
You: "Still not right. What about approach C?"
AI: "Here's approach C..."
[approach C works]
You: "Great, let's go with that"
```

The chat log shows A → B → C → success. But what's missing?

- What about the branch where you tried D?
- What about the variation of B that almost worked?
- What if you want to revisit A later with new information?

Linear history loses the tree. You only see the path you took, not the paths you could have taken.

---

## What Session Forking Looks Like

Session forking lets you branch from any point, explore independently, and merge back what works.

```
main ────●────●────●
              │
              └──●────●────● (exploration A)
                        │
                        └──● (merged back)
              │
              └──●────● (exploration B - dead end)
```

### How It Works

1. **Fork** — Create a new session branching from any event in the parent
2. **Explore** — Work independently in the forked session
3. **Evaluate** — Did the exploration work? Is it worth keeping?
4. **Merge or discard** — Merge valuable branches back, discard dead ends

The history preserves the entire tree. You can always see what was explored, even if it wasn't merged.

---

## When to Fork

| Scenario | Why Fork |
|----------|----------|
| **Alternative approaches** | "Let me try this a different way" |
| **Risky experiments** | "What if I refactor this? Let me branch first" |
| **Parallel debugging** | "You try X, I'll try Y, we'll compare" |
| **Second opinions** | "Let me ask GPT-4 the same question, see if it agrees" |
| **Time travel** | "What if we had made a different decision at step 5?" |
| **Code review** | "Review this approach without affecting main session" |

---

## A Real Example

You're debugging a flaky test.

**Main Session (Session A)**
```
[14:00] You: "The auth test is flaky. Let's investigate."
[14:05] Claude: "I see three potential causes. Let's explore each."
[14:10] Claude: "Cause 1: race condition in token refresh"
[14:15] Claude: "Let's add logging to confirm..."
[14:20] Claude: "Logs show the race condition. Here's a fix."
```

**Fork: Second Opinion (Session B)**
```
[Forked from 14:05]
[14:25] You: "Claude thinks it's a race condition. Do you agree?"
[14:26] GPT-4: "Let me analyze the same code..."
[14:30] GPT-4: "I see the race condition, but I also notice a timing issue 
              in the mock server. That could be the real cause."
[14:35] You: "Interesting. Let me verify the mock timing."
```

**Fork: Alternative Fix (Session C)**
```
[Forked from 14:20]
[14:40] You: "Claude's fix works, but let me try a different approach—
              use a mutex instead of a channel."
[14:45] Claude: "Here's the mutex-based fix..."
[14:50] You: "This is cleaner. Let's use this instead."
```

**Result: Merged Session**
```
[14:00] Initial investigation
[14:05] Three potential causes identified
[14:10-14:20] Explored race condition (merged)
[14:25-14:35] GPT-4 found additional issue (merged - critical insight)
[14:40-14:50] Tried alternative fix (merged - cleaner solution)
```

The final history shows:
- What was tried
- What was discovered
- What was merged
- What was discarded

---

## Cross-Tool Forking

Here's where it gets powerful: forks can use different AI tools.

```
Session A (Claude)
    │
    ├── Session B (GPT-4) ─── "Second opinion on diagnosis"
    │
    ├── Session C (Gemini) ── "Review with larger context"
    │
    └── Session D (Claude) ── "Continue with original tool"
```

Each fork is a full session with its own event stream. But they're all connected. You can trace any insight back to its origin.

### The Workflow

1. Claude reasons through architecture → Session A
2. Fork to GPT-4 → Session B (sees Session A's history)
3. GPT-4 proposes alternative → Session B continues
4. Merge B's insights back to A → Session A now includes both perspectives

You're not choosing between tools. You're orchestrating them.

---

## The Mechanics

### Creating a Fork

```bash
# Fork from the current position
wsm session fork --workspace my-project --session session-a --name exploration-b

# Fork from a specific event
wsm session fork --workspace my-project --session session-a --at-event 01JX2K3M... --name exploration-c
```

### Listing Branches

```bash
wsm session tree --workspace my-project

session-a (main)
├── session-b (exploration-b) [active]
│   └── session-d (deep-dive) [merged]
└── session-c (exploration-c) [discarded]
```

### Merging Back

```bash
# Merge session-b into session-a
wsm session merge --workspace my-project --from session-b --into session-a
```

The merge creates new events in the target session, linking back to the source. The fork isn't deleted—it remains as historical context.

### Discarding

```bash
# Mark session-c as discarded (kept for history, but not merged)
wsm session discard --workspace my-project --session session-c
```

Discarded sessions remain in the event store. They're valuable as a record of what didn't work.

---

## What Gets Preserved

| Preserved | Why It Matters |
|-----------|----------------|
| Fork point | Where the branch started |
| Full session tree | Every exploration, even discarded ones |
| Merge events | When and why branches were merged |
| Tool attribution | Which AI did what |
| Device attribution | Where work happened |

You can reconstruct not just what was decided, but the entire decision tree.

---

## The Analogy: Git for Conversations

If you know Git, this pattern is familiar:

| Git Concept | Session Equivalent |
|-------------|-------------------|
| Branch | Forked session |
| Commit | Event |
| Merge | Merge session |
| Rebase | (Not recommended for sessions) |
| Cherry-pick | Import specific events |
| Log | Event timeline |
| Diff | Compare sessions |

Sessions are to conversations what Git is to code. Version control for your thinking.

### Key Difference: No Fast-Forward

Unlike Git, session merges always preserve branch history. There's no "fast-forward"—the fork remains visible as a fork. This is intentional. The exploration path is as valuable as the result.

---

## Anti-Patterns

| Anti-Pattern | Problem |
|--------------|---------|
| **Never forking** | Linear history, lost alternatives |
| **Forking everything** | Unmanageable tree, no clear main line |
| **Not merging** | Insights trapped in branches |
| **Deleting forks** | Loses record of what didn't work |
| **Forking without context** | New session doesn't see parent history |

### The Discipline

Forking works best with intention:

1. **Fork with purpose** — Know why you're branching
2. **Keep main clean** — Main session represents the current best path
3. **Merge valuable insights** — Don't let good work stay isolated
4. **Preserve dead ends** — They're valuable for future reference

---

## What's Next

You can now branch and merge sessions across tools. But you also work across machines—laptop, desktop, cloud VM.

Next post: The device problem—how to maintain session continuity across physical boundaries.

[Continue to Part 5: The Device Problem →](05-the-device-problem.md)

---

*This post is part of the Cross-IDE Sessions series. [← Back to Part 3](03-why-your-ide-shouldnt-own-your-history.md)*
