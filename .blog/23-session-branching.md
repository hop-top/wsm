# Session Branching: Git for Conversations

*Part 3 of the WSM Standalone series*

---

Debugging isn't linear. Design isn't linear. You explore multiple paths, hit dead ends, backtrack, try again.

Your session history should reflect that reality—not flatten it into a single timeline that never existed.

---

## The Problem: Linear History for Non-Linear Work

Most chat logs are linear. Message after message, in sequence:

```
You: "Let's try approach A"
AI: "Here's approach A..."
You: "That didn't work. Let me try B"
AI: "Here's approach B..."
You: "Still not right. What about C?"
AI: "Here's approach C..."
You: "Great, let's go with that"
```

The log shows A → B → C → success. But what's missing?

- What about the branch where you tried approach D?
- What about the variation of B that almost worked?
- What if you want to revisit A with new information?
- What did you rule out and why?

Linear history loses the tree. You only see the path you took.

---

## The Git Analogy

If you know Git, you understand branching:

```
main ────●────●────●
              │
              └──●────●────● (feature branch)
                        │
                        └──● (merged back)
```

Git lets you:
- **Branch** — Try something without affecting main
- **Explore** — Work independently in the branch
- **Merge** — Bring good work back
- **Preserve** — Keep the history of what was tried

WSM brings this to conversations.

---

## Session Branching in WSM

```
session-a ──┬──▶ session-a (continued)
            │
            ├──▶ session-b (forked exploration)
            │         │
            │         └──▶ session-d (merged back)
            │
            └──▶ session-c (discarded dead end)
```

### How It Works

1. **Fork** — Create a new session branching from any point
2. **Explore** — Work independently in the forked session
3. **Evaluate** — Did it work? Is it worth keeping?
4. **Merge or discard** — Bring value back, or mark as explored

The history preserves the entire tree—not just the result, but every path explored.

---

## When to Branch

| Scenario | Why Branch |
|----------|------------|
| Alternative approaches | "Let me try this differently" |
| Risky experiments | "What if I refactor? Let me branch first" |
| Parallel debugging | "You try X, I'll try Y, we'll compare" |
| Second opinions | "Let me ask another model the same question" |
| Time travel | "What if we had made a different decision at step 5?" |
| Safe exploration | "I want to explore without polluting the main session" |

---

## The Commands

### Creating a Fork

```bash
# Fork from current position
wsm session fork my-project --session session-a --name exploration-b

# Fork from a specific event
wsm session fork my-project --session session-a --at-event 01JX2K3M... --name what-if-c
```

### Listing Branches

```bash
wsm session tree my-project --session session-a

session-a (main)
├── session-b (exploration-b) [active]
│   └── session-d (deep-dive) [merged]
└── session-c (what-if-c) [discarded]
```

### Merging Back

```bash
# Merge session-b into session-a
wsm session merge my-project --from session-b --into session-a
```

The merge creates new events in the target session, linking back to the source. The fork isn't deleted—it remains as historical context.

### Discarding

```bash
# Mark session-c as discarded
wsm session discard my-project --session session-c
```

Discarded sessions remain in the event store. They're valuable as a record of what didn't work.

---

## A Real Example

You're debugging a flaky test.

**Main Session (session-a)**

```bash
wsm session show session-a
```

```
[14:00] You: "The auth test is flaky. Let's investigate."
[14:05] Claude: "I see three potential causes. Let's explore each."
[14:10] Claude: "Cause 1: race condition in token refresh"
[14:15] Claude: "Let's add logging to confirm..."
[14:20] Claude: "Logs show the race condition. Here's a fix."
```

**Fork: Second Opinion (session-b)**

```bash
wsm session fork --session session-a --at-event 14:05 --name second-opinion
```

```
[Forked from 14:05]
[14:25] You: "Claude thinks it's a race condition. Do you agree?"
[14:26] GPT-4: "Let me analyze the same code..."
[14:30] GPT-4: "I see the race condition, but I also notice a timing
              issue in the mock server. That could be the real cause."
[14:35] You: "Interesting. Let me verify the mock timing."
```

**Fork: Alternative Fix (session-c)**

```bash
wsm session fork --session session-a --at-event 14:20 --name alt-fix
```

```
[Forked from 14:20]
[14:40] You: "Claude's fix works, but let me try a mutex instead of a channel."
[14:45] Claude: "Here's the mutex-based fix..."
[14:50] You: "This is cleaner. Let's use this instead."
```

**Result: Merged History**

```bash
wsm session tree session-a
```

```
session-a (main)
├── session-b (second-opinion) [merged]
│   └── Found additional mock timing issue
└── session-c (alt-fix) [merged]
    └── Cleaner mutex-based solution adopted
```

The final history shows what was tried, what was discovered, what was merged, what was discarded.

---

## Why This Matters

| Without Branching | With Branching |
|-------------------|----------------|
| One linear path | Full exploration tree |
| Lost alternatives | Preserved alternatives |
| No "what if" | Time travel to any decision point |
| Polluted main history | Clean main with branches |
| No record of failures | Failures become learning |

### The Value of Failed Branches

Discarded branches aren't waste. They're knowledge:

- "We tried X and it didn't work because Y"
- "Approach A was considered but rejected due to Z"
- "The mutex solution was cleaner than channels"

When someone asks "why didn't we do X?", you have the answer—not from memory, from history.

---

## Session Branching vs Git Branching

| Aspect | Git | WSM Sessions |
|--------|-----|--------------|
| Unit | Code | Conversation |
| Branch | Copy of files | Fork of event stream |
| Merge | Combine code | Append events with references |
| Conflict | Manual resolution | Timestamp ordering |
| History | Commits | Events |
| Discard | Delete branch | Mark as explored |

### Key Difference: No Conflicts

Conversations don't conflict like code. WSM merges by appending events with timestamps. The source branch is preserved as context.

---

## The Discipline

Branching works best with intention:

1. **Branch with purpose** — Know why you're forking
2. **Name branches descriptively** — `second-opinion`, `alt-fix`, `explore-mutex`
3. **Keep main clean** — Main session represents the current best path
4. **Merge valuable insights** — Don't let good work stay isolated
5. **Preserve dead ends** — They're valuable for future reference

---

## What's Next

You can now branch and merge sessions. But why does this matter beyond convenience?

Next post: The Audit Trail—why you need it and how simple it becomes when you have it.

[Continue to Part 4: The Audit Trail →](21-the-audit-trail-you-needed.md)

---

*This post is part of the WSM Standalone series. [← Back to Part 2](22-import-everything.md)*
