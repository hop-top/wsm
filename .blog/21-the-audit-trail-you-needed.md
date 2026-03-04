# The Audit Trail You Didn't Know You Needed

*Part 4 of the WSM Standalone series*

---

Most developers don't think about audit trails until they desperately need one.

Then it's too late.

---

## The Questions You Can't Answer

| Question | Your Current Answer |
|----------|---------------------|
| "Why did we make this decision?" | "I think it was because... maybe check Slack?" |
| "Who changed this?" | Git blame (but the reasoning is gone) |
| "What did we try before this worked?" | Memory, if you're lucky |
| "How did this bug get introduced?" | Bisect and guess |
| "Did we consider security?" | "I think so?" |
| "What was the alternative we rejected?" | No idea |

These questions come up constantly. And you can't answer them because the context lives in chat logs you can't search, in memories that fade, in people who leave.

---

## What an Audit Trail Actually Is

An audit trail isn't about surveillance. It's about memory.

| Memory Type | Where It Lives Today | Where It Should Live |
|-------------|---------------------|---------------------|
| Code history | Git | Git (good) |
| Decision history | Lost / Tribal / Docs | Event store |
| Exploration history | Lost | Session tree |
| Reasoning history | Lost | Message events |
| Tool usage history | Lost | Tool call events |
| Cross-tool context | Lost | Unified history |

Git tells you what changed. An audit trail tells you why.

---

## The Simplicity When You Need It

The power isn't in having an audit trail. It's in how simple it becomes to answer questions you couldn't answer before.

### Scenario 1: The Old Bug

```
PM: "This bug was reported. When did it start? What were we thinking?"
```

**Without an audit trail:**

1. Check git history
2. Find commit that touched the code
3. Read commit message: "fix stuff" (useless)
4. Dig through PR comments
5. Search Slack history
6. Ask around
7. Give up, just fix the bug

**With WSM:**

```bash
# Find changes to the problematic file
wsm events list my-project --type file.changed --path "src/auth.go"

# See the full context around that change
wsm session show <session-id> --around-event <event-id>
```

Output shows the entire conversation: what was discussed, what alternatives were considered, the reasoning that led to the change, and the "this should be fine" that preceded the bug.

### Scenario 2: Onboarding

```
New dev: "Why are we using library X instead of Y?"
```

**Without an audit trail:**

- "Ask Bob, he might remember"
- Check the architecture docs (outdated)
- Check the ADR (if one exists, if it's current)
- Guess

**With WSM:**

```bash
# Find every session where libraries were discussed
wsm events search my-project "library" --type message.added

# Export the relevant session
wsm session export <session-id> --format markdown
```

Hand the new dev the full decision-making process. Not a summary. The actual conversation.

### Scenario 3: The Compliance Audit

```
Auditor: "Show me who accessed production credentials and when."
```

**Without an audit trail:**

- Scramble through access logs
- Different systems, different formats
- Gaps in coverage
- Hope nothing is missing
- Manually correlate data

**With WSM:**

```bash
# Every time production files were accessed
wsm events list my-project \
  --type tool_call.completed \
  --tool "read_file" \
  --path "*production*"

# Export for the auditor
wsm export my-project --format json --since "Q4 2024"
```

Complete. Machine-readable. Defensible.

### Scenario 4: Post-Incident Review

```
You: "How did we end up with this architecture? What were the constraints?"
```

**Without an audit trail:**

- Memory fades
- People leave
- Context is lost
- "We've always done it this way"
- Repeat the same mistakes

**With WSM:**

```bash
# See the entire exploration tree
wsm session tree my-project

# Find architectural discussions
wsm events list my-project --type message.added --contains "architecture"
```

The session tree shows every path explored, every branch discarded, every reason for the final choice.

---

## The Simplicity Principle

The best audit trail is one you don't think about until you need it.

| Attribute | WSM Approach |
|-----------|--------------|
| **Automatic** | Events recorded as you work. No manual logging. No "remember to document." |
| **Unified** | One place for all tools, all sessions, all devices. Not scattered across vendors. |
| **Queryable** | CLI, SQL, JSON export. Pick your interface. |
| **Replayable** | Reconstruct any point in time. See what was known when. |
| **Yours** | Local SQLite. Not in someone else's cloud. Not dependent on a vendor. |

### The Anti-Pattern: Building It After the Fact

You can't retroactively create an audit trail. The moment you need it is the moment you realize you should have had it all along.

The cost of building it later:

| Approach | Effort | Coverage |
|----------|--------|----------|
| Interviews | High | Incomplete, biased |
| Git archaeology | Medium | Only code changes |
| Slack search | Medium | Fragmented, expires |
| Documentation | High | Always outdated |

The cost of having it from the start:

| Approach | Effort | Coverage |
|----------|--------|----------|
| WSM | Zero (automatic) | Complete |

---

## What Gets Recorded

Every event in WSM is an audit trail entry:

```json
{
  "id": "01JX2K3M4N5P6Q7R8S9T",
  "workspace_id": "01JX1H2G3F4E5D6C7B8A",
  "session_id": "01JX1J2I3H4G5F6E7D8C",
  "type": "message.added",
  "device_id": "a1b2c3d4e5f6...",
  "agent_id": "claude-3-sonnet",
  "timestamp": "2025-02-20T14:32:00Z",
  "seq_num": 127,
  "data": {
    "role": "assistant",
    "content": "Based on the security review, I recommend...",
    "references": ["event_id_1", "event_id_2"]
  }
}
```

This single event tells you:

- **What** happened (message added)
- **When** it happened (timestamp, sequence number)
- **Where** it happened (workspace, session)
- **Who** caused it (agent, device)
- **Why** (the content, the references to prior context)

Stitched together, events form a complete narrative.

---

## The Queries You Can Run

```bash
# What did I work on last week?
wsm events list my-project --since "1 week ago"

# What files were changed by Claude?
wsm events list my-project --type file.changed --agent claude

# What happened on the laptop vs desktop?
wsm events list my-project --device laptop
wsm events list my-project --device desktop

# What were we discussing when we made this decision?
wsm session show <session-id> --around-timestamp "2025-02-15T14:00:00Z"

# Export everything for archival
wsm export my-project --format json --output audit-2024-q4.json
```

Or query directly with SQL:

```sql
SELECT timestamp, agent_id, type, json_extract(data, '$.content')
FROM events
WHERE workspace_id = 'my-project'
  AND timestamp BETWEEN '2024-10-01' AND '2024-12-31'
ORDER BY seq_num;
```

---

## The Value Proposition

| Without Audit Trail | With Audit Trail |
|--------------------|------------------|
| Decisions are mysteries | Decisions are traceable |
| Context is tribal knowledge | Context is preserved |
| Onboarding is slow | Onboarding is self-serve |
| Incidents are guesses | Incidents are investigations |
| Compliance is panic | Compliance is export |
| History is lost | History is yours |

The complexity of building an audit trail after the fact is enormous. The simplicity of having one already there is transformative.

You don't need it every day. But when you need it, nothing else will do.

---

## What's Next

You now understand why audit trails matter. But audit trails are only useful if they span all your tools.

Next post: Cross-machine continuity—how to maintain one history across laptop, desktop, and cloud.

[Continue to Part 5: Cross-Machine Continuity →](05-cross-machine-continuity.md)

---

*This post is part of the WSM Standalone series. [← Back to Part 3](03-session-branching.md)*
