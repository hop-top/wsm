# Session Management Sequence Diagrams

This document contains sequence diagrams for the session management feature.

## Table of Contents

1. [Session Create](#session-create)
2. [Session Fork](#session-fork)
3. [Tree Navigation](#tree-navigation)
4. [One-Level Rebase](#one-level-rebase)
5. [Recursive Rebase](#recursive-rebase)
6. [Clone Remote Session](#clone-remote-session)
7. [Session Sync](#session-sync)

---

## Session Create

```mermaid
sequenceDiagram
    autonumber
    participant CLI as CLI / TUI
    participant SM as SessionManager
    participant JSONL as JSONL Storage
    participant IDX as SQLite Index
    participant META as Metadata

    CLI->>SM: NewSession(workspaceID, opts)
    activate SM

    SM->>SM: Generate ULID for session ID
    SM->>SM: Create session directory

    %% Create metadata
    SM->>META: Initialize metadata.json
    activate META
    META-->>SM: Metadata created
    deactivate META

    %% Initialize JSONL file
    SM->>JSONL: Create session.jsonl
    activate JSONL
    JSONL-->>SM: File created
    deactivate JSONL

    %% Initialize SQLite index
    SM->>IDX: Create index.db with schema
    activate IDX
    IDX->>IDX: Create entries table
    IDX-->>SM: Index initialized
    deactivate IDX

    SM->>SM: Create Session struct

    %% Check if in-memory mode
    alt opts.Storage == Memory
        SM->>SM: Initialize MemoryStorage
    else opts.Storage == File
        SM->>JSONL: Set file path
    end

    SM-->>CLI: Session created
    deactivate SM
```

---

## Session Fork

```mermaid
sequenceDiagram
    autonumber
    participant CLI as CLI / TUI
    participant SM as SessionManager
    participant JSONL as JSONL Storage
    participant IDX as SQLite Index
    participant LLM as LLM Provider (opt)
    participant META as Metadata

    CLI->>SM: ForkSession(sessionID, entryID, opts)
    activate SM

    %% Step 1: Load parent session
    SM->>META: GetSession(sessionID)
    activate META
    META-->>SM: ParentSession
    deactivate META

    %% Step 2: Get entries from fork point to current
    SM->>IDX: GetEntriesByParent(sessionID, null)
    activate IDX
    IDX->>IDX: Query all root entries
    IDX-->>SM: Root entries
    deactivate IDX

    loop For each child branch
        SM->>IDX: GetEntryByID(entryID)
        IDX-->>SM: Entry details
    end

    %% Step 3: Generate branch summary
    alt opts.UseLLM == true && LLM available
        SM->>LLM: GenerateSummary(divergedEntries)
        activate LLM
        LLM->>LLM: Call OpenAI/Anthropic API
        LLM-->>SM: Summary text
        deactivate LLM
    else opts.UseLLM == false
        SM->>SM: GenerateTemplateSummary(divergedEntries)
    end

    %% Step 4: Create new session metadata
    SM->>SM: Generate ULID for new session ID
    SM->>SM: Create session directory
    SM->>META: Create metadata.json
    activate META
    META->>META: Set parent_session = sessionID
    META-->>SM: New session metadata
    deactivate META

    %% Step 5: Initialize storage
    SM->>JSONL: Create new session.jsonl
    activate JSONL
    JSONL-->>SM: File created
    deactivate JSONL

    SM->>IDX: Create index.db
    activate IDX
    IDX-->>SM: Index initialized
    deactivate IDX

    %% Step 6: Append BranchSummary entry
    SM->>SM: Create BranchSummaryEntry
    Note over SM: Entry: {<br/>  type: "branch_summary",<br/>  parent_id: entryID,<br/>  content: { summary_text, fork_point }<br/>}

    SM->>JSONL: writeJSONLine(BranchSummaryEntry)
    activate JSONL
    JSONL->>JSONL: Atomic append
    JSONL-->>SM: Entry appended
    deactivate JSONL

    SM->>IDX: indexEntry(BranchSummaryEntry)
    activate IDX
    IDX-->>SM: Entry indexed
    deactivate IDX

    %% Step 7: Update fork chain metadata
    SM->>META: Update fork_chain, fork_depth
    activate META
    META-->>SM: Metadata updated
    deactivate META

    SM-->>CLI: NewSession (forked)
    deactivate SM
```

---

## Tree Navigation

```mermaid
sequenceDiagram
    autonumber
    participant CLI as CLI / TUI
    participant SM as SessionManager
    participant EVT as Event Emitter
    participant JSONL as JSONL Storage
    participant IDX as SQLite Index
    participant CTX as Context Builder

    CLI->>SM: PrepareTree(sessionID)
    activate SM

    %% Emit pre-navigation event
    SM->>EVT: Emit "session_before_tree"
    activate EVT
    EVT-->>SM: Event emitted (cancellable)
    deactivate EVT

    %% Build tree structure
    SM->>IDX: GetEntryByID(sessionID, rootEntryID)
    activate IDX
    IDX-->>SM: Root entry
    deactivate IDX

    loop For each entry in tree
        SM->>IDX: GetEntriesByParent(sessionID, entryID)
        IDX-->>SM: Children entries
        SM->>SM: Build TreeNode with children
    end

    SM->>SM: Calculate currentPath from root to current
    SM-->>CLI: []TreeNode, []currentPath
    deactivate SM

    %% Navigate to specific entry
    CLI->>SM: NavigateToEntry(sessionID, targetEntryID)
    activate SM

    SM->>IDX: GetEntryByID(targetEntryID)
    activate IDX
    IDX-->>SM: Target entry
    deactivate IDX

    %% Replay entries up to target
    SM->>CTX: BuildContext(sessionID, upTo: targetEntryID)
    activate CTX

    loop Replay entries from root to target
        CTX->>JSONL: readJSONLine(session.jsonl)
        activate JSONL
        JSONL-->>CTX: Entry data
        deactivate JSONL

        alt Entry.type == "message"
            CTX->>CTX: Add message to context
        else Entry.type == "file_edit"
            CTX->>CTX: Apply file diff
        else Entry.type == "terminal_cmd"
            CTX->>CTX: Record command in context
        end
    end

    CTX-->>SM: SessionContext (messages, files, state)
    deactivate CTX

    %% Emit post-navigation event
    SM->>EVT: Emit "session_tree"
    activate EVT
    EVT-->>SM: Event emitted
    deactivate EVT

    SM-->>CLI: SessionContext
    deactivate SM
```

---

## One-Level Rebase

```mermaid
sequenceDiagram
    autonumber
    participant CLI as CLI / TUI
    participant SM as SessionManager
    participant IDX as SQLite Index
    participant JSONL as JSONL Storage
    participant CONFLICT as Conflict Resolver
    participant META as Metadata

    CLI->>SM: RebaseWithChild(parentID, childID)
    activate SM

    %% Step 1: Verify child is immediate fork of parent
    SM->>META: GetSession(parentID)
    activate META
    META-->>SM: ParentSession
    deactivate META

    SM->>META: GetSession(childID)
    activate META
    META-->>SM: ChildSession
    deactivate META

    SM->>SM: Validate: Child.parent_session == parentID

    %% Step 2: Get diverged entries
    SM->>IDX: GetEntriesByParent(parentID, child.forkPoint)
    activate IDX
    IDX-->>SM: Parent's diverged entries
    deactivate IDX

    SM->>IDX: GetEntriesByParent(childID, child.forkPoint)
    activate IDX
    IDX-->>SM: Child's diverged entries
    deactivate IDX

    %% Step 3: Detect conflicts
    SM->>CONFLICT: DetectConflicts(parentEntries, childEntries)
    activate CONFLICT

    loop Compare entries
        CONFLICT->>CONFLICT: Check file path conflicts
        CONFLICT->>CONFLICT: Check message conflicts
    end

    alt Conflicts found
        CONFLICT-->>SM: ConflictReport
        deactivate CONFLICT

        %% Prompt user for resolution
        SM->>CLI: Show conflict report
        CLI-->>SM: Resolution choice (keep-parent|keep-child|merge-manual)
    else No conflicts
        CONFLICT-->>SM: No conflicts
        deactivate CONFLICT
    end

    %% Step 4: Apply rebase
    alt Resolution == keep-child
        SM->>JSONL: Append child entries to parent
        activate JSONL
        loop For each child entry
            JSONL->>JSONL: Atomic append
        end
        JSONL-->>SM: Entries appended
        deactivate JSONL

        SM->>IDX: Index appended entries
        activate IDX
        IDX-->>SM: Entries indexed
        deactivate IDX
    else Resolution == keep-parent
        SM->>SM: Skip child entries
    else Resolution == merge-manual
        SM->>SM: User merges manually
        SM->>JSONL: Append merged entries
        activate JSONL
        JSONL-->>SM: Merged entries
        deactivate JSONL
    end

    %% Step 5: Create rebase summary entry
    SM->>SM: Create RebaseSummaryEntry
    Note over SM: Entry: {<br/>  type: "rebase_summary",<br/>  content: { rebase_from, rebase_to, conflicts_resolved }<br/>}

    SM->>JSONL: writeJSONLine(RebaseSummaryEntry)
    activate JSONL
    JSONL-->>SM: Entry appended
    deactivate JSONL

    SM->>IDX: indexEntry(RebaseSummaryEntry)
    activate IDX
    IDX-->>SM: Entry indexed
    deactivate IDX

    %% Step 6: Update fork pointers
    SM->>META: Update parent fork pointer
    activate META
    META->>META: Set last_fork_from = childID
    META-->>SM: Metadata updated
    deactivate META

    SM-->>CLI: Rebase complete
    deactivate SM
```

---

## Recursive Rebase

```mermaid
sequenceDiagram
    autonumber
    participant CLI as CLI / TUI
    participant SM as SessionManager
    participant CHAIN as Fork Chain Tracker
    participant IDX as SQLite Index
    participant REBASE as One-Level Rebase

    CLI->>SM: RecursiveRebase(sessionID, targetID)
    activate SM

    %% Step 1: Build fork chain
    SM->>CHAIN: GetForkChain(sessionID)
    activate CHAIN

    loop Walk up parent chain
        CHAIN->>IDX: GetSession(parentID)
        activate IDX
        IDX-->>CHAIN: Session
        deactivate IDX

        CHAIN->>CHAIN: Add to chain
    end

    CHAIN-->>SM: ForkChain [sessionID, parent1, parent2, ..., root]
    deactivate CHAIN

    %% Step 2: Find rebase path
    SM->>SM: Check if targetID is in fork chain

    alt targetID == immediate child
        SM->>REBASE: RebaseWithChild(sessionID, targetID)
        activate REBASE
        REBASE-->>SM: Rebase complete
        deactivate REBASE
    else targetID is deeper in chain
        %% Recursive rebase through intermediate parents
        SM->>SM: Identify intermediate parents between sessionID and targetID

        loop For each intermediate parent
            SM->>REBASE: RebaseWithChild(parent, child)
            activate REBASE
            REBASE-->>SM: Rebase complete
            deactivate REBASE

            alt Conflicts detected
                SM->>CLI: Report conflict and pause
                CLI-->>SM: Continue or abort
            end
        end
    else targetID not in chain
        SM-->>CLI: Error: targetID not reachable
        deactivate SM
    end

    %% Step 3: Final summary
    SM->>SM: Generate rebase summary
    SM-->>CLI: Recursive rebase complete
    deactivate SM
```

---

## Clone Remote Session

```mermaid
sequenceDiagram
    autonumber
    participant CLI as CLI / TUI
    participant RC as Remote Client
    participant APS as APS Protocol Server
    participant SM as SessionManager
    participant JSONL as JSONL Storage
    participant IDX as SQLite Index
    participant META as Metadata

    CLI->>RC: ListRemoteSessions(protocolURL)
    activate RC

    RC->>APS: GET /sessions
    activate APS
    APS-->>RC: []RemoteSessionMetadata
    deactivate APS

    RC-->>CLI: Available remote sessions
    deactivate RC

    %% User selects session to clone
    CLI->>RC: CloneRemoteSession(protocolURL, remoteSessionID)
    activate RC

    RC->>APS: GET /sessions/{remoteSessionID}
    activate APS
    APS-->>RC: RemoteSessionDetails + JSONL URL
    deactivate APS

    %% Download session data
    RC->>APS: Download JSONL file
    activate APS
    APS-->>RC: JSONL content
    deactivate APS

    %% Step 1: Auto-fork on clone
    RC->>SM: NewSession(workspaceID, opts)
    activate SM

    SM->>SM: Generate local session ID
    SM->>META: Create metadata with remote origin
    activate META
    META->>META: Set remote_origin = protocolURL<br/>Set remote_node_url = session URL
    META-->>SM: Local session created
    deactivate META
    deactivate SM

    %% Step 2: Import remote entries
    RC->>JSONL: Import JSONL content
    activate JSONL
    JSONL->>JSONL: Write entries to local session.jsonl
    JSONL-->>RC: Import complete
    deactivate JSONL

    %% Step 3: Build index from imported entries
    RC->>IDX: Rebuild index from JSONL
    activate IDX

    loop For each entry in JSONL
        IDX->>IDX: Parse entry
        IDX->>IDX: Insert into entries table
    end

    IDX-->>RC: Index rebuilt
    deactivate IDX

    %% Step 4: Create auto-fork summary
    RC->>SM: AppendEntry(ForkSummaryEntry)
    activate SM
    SM->>JSONL: writeJSONLine(ForkSummaryEntry)
    activate JSONL
    JSONL-->>SM: Entry appended
    deactivate JSONL

    SM->>IDX: indexEntry(ForkSummaryEntry)
    activate IDX
    IDX-->>SM: Entry indexed
    deactivate IDX
    deactivate SM

    RC-->>CLI: Local session (auto-forked)
    deactivate RC
```

---

## Session Sync

```mermaid
sequenceDiagram
    autonumber
    participant CLI as CLI / TUI
    participant SM as SessionManager
    participant RC as Remote Client
    participant APS as APS Protocol Server
    participant META as Metadata
    participant IDX as SQLite Index

    CLI->>SM: GetSession(localSessionID)
    activate SM

    SM->>META: GetSession(localSessionID)
    activate META
    META-->>SM: Session with remote_origin
    deactivate META
    deactivate SM

    SM->>RC: SyncSession(localSession, remoteOrigin)
    activate RC

    %% Step 1: Get local stats
    RC->>IDX: GetStats(localSessionID)
    activate IDX
    IDX-->>RC: LocalStats (entryCount, lastUpdate)
    deactivate IDX

    %% Step 2: Get remote stats
    RC->>APS: HEAD /sessions/{remoteSessionID}
    activate APS
    APS-->>RC: RemoteStats (entryCount, lastUpdate)
    deactivate APS

    %% Step 3: Compare
    RC->>RC: Compare local vs remote stats

    alt local.lastUpdate > remote.lastUpdate
        %% Local has new changes
        RC-->>CLI: Local ahead of remote<br/>Options: Push changes, create new fork
    else remote.lastUpdate > local.lastUpdate
        %% Remote has new changes
        RC-->>CLI: Remote ahead of local<br/>Options: Pull changes, create new fork
    else entryCounts differ
        %% Diverged histories
        RC-->>CLI: Sessions diverged<br/>Options: Rebase, create merge fork
    else Stats match
        RC-->>CLI: Sessions are in sync
    end

    %% User chooses to pull changes
    CLI->>RC: Pull changes (create fork)
    activate RC

    RC->>APS: GET /sessions/{remoteSessionID}/entries?from={local.lastUpdate}
    activate APS
    APS-->>RC: New remote entries
    deactivate APS

    %% Append as new fork
    RC->>SM: NewSession(workspaceID, opts)
    activate SM
    SM-->>RC: New fork session created
    deactivate SM

    RC->>SM: AppendEntries(newRemoteEntries)
    activate SM
    SM-->>RC: Entries appended
    deactivate SM

    RC-->>CLI: Sync complete (new fork created)
    deactivate RC
```
