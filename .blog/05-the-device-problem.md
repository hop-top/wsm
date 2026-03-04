# The Device Problem: Your Session, Everywhere

*Part 5 of the Cross-IDE Sessions series*

---

You don't code on one machine. You have a laptop for coffee shops, a desktop for deep work, maybe a cloud VM for heavy lifting. Each machine is a silo—its own sessions, its own context, its own amnesia.

The device problem is simple: how do you maintain continuity when you're never in one place?

---

## The Current Reality: Machine Boundaries

Today, your coding sessions are machine-bound:

| Where | What Happens |
|-------|--------------|
| **Laptop** | Morning exploration, architectural thinking |
| **Desktop** | Afternoon implementation, deep focus |
| **Cloud VM** | Heavy computation, production access |
| **Work machine** | Day job context |
| **Personal machine** | Side projects |

Each machine has its own history. When you switch, you start over.

### What You Lose

| Lost | Impact |
|------|--------|
| Context from laptop | Desktop doesn't know what you explored this morning |
| Train of thought | You have to rebuild mental state |
| Session continuity | The conversation is fragmented by geography |
| Audit trail | Can't see the full picture across machines |

You might remember what you were doing. Your tools don't.

---

## The WSM Approach: Device-Aware, Not Device-Bound

WSM solves this differently. Instead of binding sessions to a machine, it:

1. **Identifies each device** deterministically
2. **Tags every event** with the device that produced it
3. **Shares the event store** across devices (via your chosen method)
4. **Maintains continuity** regardless of where work happens

The device becomes metadata, not a boundary.

---

## Device Identification

### How It Works

Each device is identified by a deterministic hash:

```
device_id = SHA-256(OS + Arch + Hostname)
```

| Component | Example |
|-----------|---------|
| OS | `darwin`, `linux`, `windows` |
| Arch | `arm64`, `amd64` |
| Hostname | `macbook-pro.local`, `dev-server-01` |

Same machine always produces the same ID. No accounts. No login. No configuration.

### Device Registry

The workspace maintains a device registry:

```json
{
  "devices": [
    {
      "id": "a1b2c3d4e5f6...",
      "name": "MacBook Pro",
      "hostname": "macbook-pro.local",
      "os": "darwin",
      "arch": "arm64",
      "first_seen": "2025-01-15T09:00:00Z",
      "last_seen": "2025-02-20T14:30:00Z"
    },
    {
      "id": "f6e5d4c3b2a1...",
      "name": "Dev Desktop",
      "hostname": "dev-desktop",
      "os": "linux",
      "arch": "amd64",
      "first_seen": "2025-01-20T10:00:00Z",
      "last_seen": "2025-02-20T16:45:00Z"
    },
    {
      "id": "1234567890ab...",
      "name": "Cloud VM",
      "hostname": "prod-debug-vm",
      "os": "linux",
      "arch": "amd64",
      "first_seen": "2025-02-01T08:00:00Z",
      "last_seen": "2025-02-19T11:00:00Z"
    }
  ]
}
```

### Why This Matters

Every event carries `device_id`:

```json
{
  "id": "01JX2K3M4N5P6Q7R8S9T",
  "type": "message.added",
  "device_id": "a1b2c3d4e5f6...",
  "timestamp": "2025-02-20T14:32:00Z",
  "data": {
    "role": "assistant",
    "content": "Based on our morning exploration..."
  }
}
```

You can always answer: "Where did this happen?"

---

## Sharing the Event Store

Device identification is half the solution. The other half is sharing the data.

### Option 1: Git

The SQLite database is a single file. Git handles it well.

```bash
# In your project repo
git add .wsm/history.db
git commit -m "Session progress from laptop"
git push

# On desktop
git pull
# Continue where you left off
```

**Pros:** Version controlled, no extra infrastructure
**Cons:** Manual sync, potential merge conflicts (rare with SQLite)

### Option 2: Cloud Storage

Dropbox, iCloud, Google Drive, Syncthing—pick your poison.

```bash
# Symlink the workspace to cloud storage
ln -s ~/Dropbox/wsm/my-project ~/.wsm/workspaces/my-project
```

**Pros:** Automatic sync, no manual steps
**Cons:** Depends on third party, potential sync delays

### Option 3: Self-Hosted Sync

Run your own sync server. SQLite-to-SQLite replication.

```bash
wsm sync --workspace my-project --remote sync.example.com:8443
```

**Pros:** Full control, no third party
**Cons:** Requires infrastructure

### Option 4: No Sync (Device-Local)

Keep histories separate. Use device tracking to understand context.

```bash
wsm events list --workspace my-project --device laptop
wsm events list --workspace my-project --device desktop
```

**Pros:** Simplest, no sync issues
**Cons:** No unified history, manual context transfer

### Which Should You Use?

| If You... | Use |
|-----------|-----|
| Already use Git heavily | Git |
| Want automatic sync | Cloud storage |
| Have strong privacy requirements | Self-hosted |
| Rarely switch mid-task | No sync |

The point is: you choose. WSM doesn't impose a sync model.

---

## Continuity in Practice

### Scenario: Laptop to Desktop

**9:00 AM - MacBook Pro (laptop)**
```
wsm session start --workspace my-project --name auth-redesign

You: "Let's rethink the authentication flow"
Claude: "I'll analyze the current implementation..."
[Claude explores, proposes three alternatives]
You: "Alternative 2 looks promising. Let's explore that tomorrow."
wsm session pause
git add .wsm/history.db && git commit -m "Auth redesign exploration" && git push
```

**2:00 PM - Dev Desktop (desktop)**
```
git pull
wsm session continue --workspace my-project --session auth-redesign

[GPT-4 sees the full morning exploration]
GPT-4: "Continuing from alternative 2. I see Claude identified a potential 
        race condition. Let me verify that in the codebase..."
[GPT-4 continues, building on laptop work]
```

The context transferred seamlessly. Different machine, different AI tool, same conversation.

### Scenario: Local to Cloud VM

**11:00 AM - Local machine**
```
You: "I need to test this against production data"
Claude: "That requires production access. You should use the debug VM."
You: "Good point. Let me switch."
wsm session pause
```

**11:15 AM - Cloud VM**
```
wsm session continue --workspace my-project --session auth-redesign

[Claude sees the context, now running on VM with production access]
Claude: "Continuing. I now have access to production logs. Let me query..."
[Claude tests against real data]
```

Same session, different security context. The VM's access enables work that laptop couldn't do.

---

## The Audit Benefit

Device tracking isn't just about continuity. It's about accountability.

### Querying by Device

```bash
# What did we do on the laptop today?
wsm events list --workspace my-project --device laptop --since today

# What did the VM do last week?
wsm events list --workspace my-project --device cloud-vm --since "1 week ago"

# Cross-device session timeline
wsm events list --workspace my-project --session auth-redesign
```

Output:
```
TIME         DEVICE        AGENT      EVENT
09:00:15     laptop        claude     session.started
09:05:32     laptop        claude     message.added
09:10:45     laptop        claude     tool_call.completed
...
14:00:10     desktop       gpt-4      message.added
14:05:22     desktop       gpt-4      tool_call.completed
...
11:15:00     cloud-vm      claude     message.added
11:20:33     cloud-vm      claude     tool_call.completed
```

### Why This Matters

| Question | Answer |
|----------|--------|
| "Who made this change?" | Check `device_id` and `agent_id` |
| "Where was this tested?" | Check `device_id` |
| "Did production access get used correctly?" | Check events from VM |
| "Was this reviewed before merging?" | Check session tree for review branch |

Device attribution creates accountability. Every action is traceable.

---

## Team Considerations

When multiple people work on the same workspace:

| Scenario | What Happens |
|----------|--------------|
| Same project, different people | Each person's device is tracked separately |
| Shared machine | Different hostnames still distinguish devices |
| Pair programming | Both devices contribute to the same session |

The device registry shows who worked where:

```bash
wsm devices list --workspace my-project

DEVICE              USER         FIRST SEEN    LAST SEEN
macbook-alice       Alice        Jan 15        Feb 20 (today)
desktop-bob         Bob          Jan 18        Feb 19
vm-shared           Team         Feb 01        Feb 20 (today)
```

---

## Anti-Patterns

| Anti-Pattern | Problem | Fix |
|--------------|---------|-----|
| **No sync, no awareness** | Devices are silos | At minimum, review device-separated histories |
| **Sync without device tracking** | Lose context of where work happened | Device IDs are automatic—don't disable them |
| **Conflicting sync methods** | Git + Dropbox = corruption | Pick one method, stick to it |
| **Ignoring device context** | Can't trace production access | Review device usage in audits |

---

## The Mental Shift

The shift is from:

| Old Model | New Model |
|-----------|-----------|
| "I'm on my laptop" | "I'm continuing the session from my laptop" |
| "I need to re-explain" | "The context is already there" |
| "Where did I leave off?" | "Let me check the last event" |
| "I'll email myself notes" | "The history travels with me" |

Your workspace follows you. The device is just an access point.

---

## What's Next

You now have devices, sessions, and history all connected. But how does this actually work under the hood?

Next post: Implementation—how WSM makes cross-IDE sessions real.

[Continue to Part 6: Implementation →](06-implementation.md)

---

*This post is part of the Cross-IDE Sessions series. [← Back to Part 4](04-session-forking.md)*
