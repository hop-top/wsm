# Why Your IDE Shouldn't Own Your History

*Part 3 of the Cross-IDE Sessions series*

---

Your coding history is the most valuable artifact you produce. It contains every decision, every exploration, every dead end, every breakthrough.

And you're giving it away to vendors who don't share it back.

---

## The Current State: Vendor-Owned Memory

Every AI coding tool stores your session history. But:

| Tool | Where Your History Lives | Who Controls It |
|------|-------------------------|-----------------|
| Claude Code | Anthropic's infrastructure | Anthropic |
| Cursor | Cursor's infrastructure | Cursor |
| Copilot | Microsoft's infrastructure | Microsoft |
| Windsurf | Codeium's infrastructure | Codeium |
| Gemini | Google's infrastructure | Google |

You can't export it. You can't query it. You can't use it in another tool. It's locked in a proprietary vault you'll never see.

### What This Means in Practice

**You can't leave.** Switching tools means abandoning your history. Months of context, gone.

**You can't combine.** Claude's reasoning and GPT-4's code generation stay separate. No synergy.

**You can't audit.** Want to know why a decision was made? Good luck finding it across five different chat logs.

**You can't back up.** Your history lives on their servers, under their terms, at their mercy.

---

## The Real Cost: Fragmented Institutional Memory

This isn't about convenience. It's about the long-term value of your work.

### The Value of History

Your session history contains:

| What's Recorded | Future Value |
|-----------------|--------------|
| Requirements discussions | Why did we build this feature? |
| Architecture decisions | What alternatives did we consider? |
| Debugging sessions | How did we solve this before? |
| Code reviews | What issues were caught? |
| Dead ends explored | What didn't work and why? |

This is institutional memory. It's worth more than the code itself. Code tells you what was built. History tells you why.

### The Fragmentation Problem

When history is scattered across vendors:

| Question | Answer |
|----------|--------|
| "Why did we choose OAuth2?" | "Check Claude, no wait, maybe it was in Cursor..." |
| "What did we try before this worked?" | "I don't remember. Maybe GPT-4 explored it?" |
| "What did the security review say?" | "That was in Gemini. Let me dig through the chat." |

You end up with five partial memories instead of one complete one. None of them know about the others.

---

## The Vendor Perspective: Lock-In Is the Feature

Vendors have no incentive to share your history.

| Vendor Incentive | Result |
|------------------|--------|
| Keep you subscribed | History is trapped in their tool |
| Prevent switching | No export, no interoperability |
| Own the relationship | Your data is their asset |

This isn't malicious. It's business. But it's not aligned with your interests.

### What Happens When Vendors Change

Vendors change. Features get deprecated. Pricing shifts. Companies get acquired or shut down.

When that happens:

- **Reddit search:** "How do I export my Claude Code history?"
- **Answer:** You can't. It's gone.

Your history shouldn't depend on a vendor's continued existence or good graces.

---

## What History Ownership Looks Like

Owning your history means:

| Capability | What It Enables |
|------------|-----------------|
| **Local storage** | Your data on your machine |
| **Open format** | Readable by any tool, not just one |
| **Full export** | Take it anywhere, anytime |
| **Programmatic access** | Query, analyze, visualize |
| **Cross-tool visibility** | Every tool sees the same history |

### The WSM Approach

WSM stores history in a local SQLite database. You own the file. You control the format. You decide what happens to it.

```
~/.wsm/
└── workspaces/
    └── my-project/
        └── history.db    # Your data. Your file.
```

Any tool can read it. Any tool can write to it. No vendor permission required.

```bash
# Export to JSON
wsm export --workspace my-project --format json --output history.json

# Query with SQL
sqlite3 ~/.wsm/workspaces/my-project/history.db "SELECT * FROM events WHERE type='message.added'"

# Backup to anywhere
cp ~/.wsm/workspaces/my-project/history.db ~/backups/
```

---

## The Shift: From Consumer to Owner

This is a mindset shift.

| Old Model | New Model |
|-----------|-----------|
| Tool owns history | You own history |
| Tool is the source of truth | Event store is the source of truth |
| Tool switch = history loss | Tool switch = same history, different view |
| Vendor controls access | You control access |
| Format is proprietary | Format is open |
| Export is impossible | Export is trivial |

The tool becomes a view into your history, not the owner of it.

---

## But What About Sync?

A common question: "If history is local, how do I sync across machines?"

The answer: however you want.

| Approach | How It Works |
|----------|--------------|
| **Git** | Commit the SQLite file (works surprisingly well) |
| **Cloud storage** | Dropbox, iCloud, Syncthing—your choice |
| **Self-hosted** | Your own server, your own rules |
| **No sync** | Different machines, different histories. Device tracking keeps it clear. |

The point is: you choose. Not the vendor.

---

## The Network Effect of Shared History

When history is owned centrally but accessed by many tools, something powerful happens:

```
Claude writes ──┐
                │
GPT-4 writes ──┼──▶ Shared History ──▶ All tools see everything
                │
Gemini reads ──┘
```

Claude's architectural reasoning becomes visible to GPT-4. GPT-4's code generation becomes reviewable by Gemini. Gemini's security analysis informs future Claude sessions.

Each tool contributes. Each tool benefits. The whole exceeds the sum of parts.

This only works when history is independent of any single tool.

---

## The Long Game

Think about your coding history over years, not weeks.

| Timeframe | Vendor-Owned History | Owned History |
|-----------|---------------------|---------------|
| 1 month | Works fine | Works fine |
| 1 year | Trapped in one tool | Portable across tools |
| 5 years | At the mercy of vendor | Under your control |
| Career | Scattered, lost, irrecoverable | Complete, searchable, valuable |

Your history is your professional memory. Treat it like the asset it is.

---

## What's Next

You now understand why history ownership matters. But history isn't linear—you explore, backtrack, try alternatives.

Next post: Session forking—how to branch, explore, and merge without losing the thread.

[Continue to Part 4: Session Forking →](04-session-forking.md)

---

*This post is part of the Cross-IDE Sessions series. [← Back to Part 2](02-anatomy-of-a-session.md)*
