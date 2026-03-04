# Cross-Machine Continuity Without the Cloud

*Part 5 of the WSM Standalone series*

---

You don't code on one machine. Laptop for coffee shops, desktop for deep work, maybe a cloud VM for production access.

Each machine is its own island. Your session history fragments along hardware boundaries.

WSM bridges those boundaries—without forcing you into someone else's cloud.

---

## The Device Problem

Today, your coding sessions are bound to the machine where they happened:

| Where | What You Lose When You Switch |
|-------|------------------------------|
| Laptop → Desktop | Context from this morning's exploration |
| Desktop → VM | Can't access local debugging session |
| Work → Personal | Projects live on different machines |
| Any switch | The mental overhead of rebuilding context |

The tool doesn't follow you. The history doesn't follow you. You start over.

---

## The Cloud Solution (And Why You Might Not Want It)

The obvious answer: sync to the cloud.

But cloud sync means:

| Concern | Reality |
|---------|---------|
| Privacy | Your coding history on someone else's servers |
| Dependency | You need internet to access your own data |
| Cost | Storage, bandwidth, subscription |
| Lock-in | Yet another vendor to depend on |
| Security | Your intellectual property in transit and at rest |

For many developers, these are non-starters. Your code is proprietary. Your decisions are confidential. Your exploration is your business.

---

## WSM's Approach: Device-Aware, Not Device-Bound

WSM takes a different approach:

1. **Identify each device** deterministically
2. **Tag every event** with the device that produced it
3. **Share the event store** via your chosen method
4. **Maintain continuity** regardless of where work happens

The device becomes metadata, not a boundary.

---

## Device Identification

WSM identifies devices without accounts, login, or configuration:

```
device_id = SHA-256(OS + Arch + Hostname)
```

| Component | Example |
|-----------|---------|
| OS | `darwin`, `linux`, `windows` |
| Arch | `arm64`, `amd64` |
| Hostname | `macbook-pro.local`, `dev-server-01` |

Same machine always produces the same ID. Different machines produce different IDs.

### The Device Registry

WSM maintains a registry of devices that have contributed to a workspace:

```bash
wsm devices list my-project
```

```
DEVICE              OS       ARCH    FIRST SEEN    LAST SEEN
macbook-pro.local   darwin   arm64   Jan 15        Feb 21 (today)
dev-desktop         linux    amd64   Jan 18        Feb 19
cloud-vm-01         linux    amd64   Feb 01        Feb 20
```

Every event is tagged with `device_id`. You always know where work happened.

---

## Sync Options: You Choose

WSM doesn't impose a sync model. You pick what works for you.

### Option 1: Git

The SQLite database is a single file. Git handles it well.

```bash
# Commit your workspace
git add .wsm/history.db
git commit -m "Session progress from laptop"
git push

# On another machine
git pull
# Continue where you left off
```

| Pros | Cons |
|------|------|
| Version controlled | Manual sync |
| No extra infrastructure | Potential merge conflicts (rare) |
| Already in your workflow | |

### Option 2: Cloud Storage

Dropbox, iCloud, Google Drive, Syncthing:

```bash
# Symlink workspace to cloud storage
ln -s ~/Dropbox/wsm/my-project ~/.wsm/workspaces/my-project
```

| Pros | Cons |
|------|------|
| Automatic sync | Depends on third party |
| No manual steps | Potential sync delays |
| | Privacy considerations |

### Option 3: Self-Hosted

Your own sync server:

```bash
wsm sync my-project --remote sync.example.com:8443
```

| Pros | Cons |
|------|------|
| Full control | Requires infrastructure |
| No third party | More setup |
| Privacy guaranteed | |

### Option 4: No Sync

Keep histories separate, use device tracking for context:

```bash
wsm events list my-project --device laptop
wsm events list my-project --device desktop
```

| Pros | Cons |
|------|------|
| Simplest | No unified history |
| No sync issues | Manual context transfer |
| Maximum privacy | |

---

## Continuity in Practice

### Scenario: Morning Laptop, Afternoon Desktop

**9:00 AM — MacBook Pro**

```bash
wsm session start my-project --name auth-redesign

# Work happens
# Claude analyzes, proposes alternatives
# You explore option 2

wsm session pause
git add .wsm/history.db && git commit -m "Auth redesign AM" && git push
```

**2:00 PM — Desktop**

```bash
git pull

wsm session continue my-project --session auth-redesign

# GPT-4 sees the full morning exploration
# Continues from where you left off
# Different AI, same conversation
```

### Scenario: Local to Cloud VM

**11:00 AM — Local Machine**

```bash
# Working on feature
# Realize you need production access

wsm session pause
```

**11:15 AM — Cloud VM**

```bash
# Same workspace, different security context
wsm session continue my-project --session feature-x

# Claude sees context, now has production access
# Tests against real data
```

---

## The Audit Benefit

Device tracking isn't just continuity—it's accountability.

```bash
# What happened on each device?
wsm events list my-project --device macbook-pro.local --since today
wsm events list my-project --device cloud-vm-01 --since today

# Cross-device timeline
wsm events list my-project --session auth-redesign
```

```
TIME         DEVICE              AGENT      EVENT
09:00:15     macbook-pro.local   claude     session.started
09:05:32     macbook-pro.local   claude     message.added
14:00:10     dev-desktop         gpt-4      message.added
11:15:00     cloud-vm-01         claude     tool_call.completed
```

### Why This Matters

| Question | Answer |
|----------|--------|
| "Where was this tested?" | Check device_id |
| "Did production access get used?" | Check events from VM |
| "Who made this change?" | Device + agent attribution |
| "Was this reviewed before merging?" | Check session tree |

---

## Privacy by Design

WSM is built for privacy:

| Feature | Privacy Implication |
|---------|-------------------|
| Local SQLite | Data never leaves your machine unless you choose |
| No accounts | No identifying information sent anywhere |
| Optional sync | You control if/how data moves |
| Open format | You can inspect, export, delete at will |

Your intellectual property stays yours.

---

## What's Next

You have unified history across tools (Post 2), branching for exploration (Post 3), audit trails for accountability (Post 4), and continuity across devices (Post 5).

Next post: Putting it all together—owning your complete workflow history.

[Continue to Part 6: Owning Your Workflow →](26-owning-your-workflow.md)

---

*This post is part of the WSM Standalone series. [← Back to Part 4](21-the-audit-trail-you-needed.md)*
