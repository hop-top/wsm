# IRC Message Capture and Backfill

**ID**: 051
**Feature**: Workspace IRC
**Persona**: [Integrator](../personas/integrator.md), [External Client](../personas/external-client.md)
**Priority**: P1

## Story

As an integrator, I want every channel message recorded as a workspace event, and gaps filled when the bot was away, so that chat becomes part of the same append-only log as file patches and tool calls.

## Design

The bot sits in every template channel and records each channel `PRIVMSG` and `NOTICE` as one `interaction.message` event. Events are appended directly through the backend so the per-message agent id and the server's own timestamp are honoured rather than being replaced by the recording process's identity and clock.

`MessageData` carries the optional fields `Channel`, `Nick`, `MsgID`, and `Attribution`, all additive. `Attribution` is `account` when the server tagged the message with an account, `nick` when identity was inferred from the nickname alone, `relay` for a bridge-relayed message, or `none`.

### Degradation

Capture never refuses to run because a capability is missing; it records what it can and says what it lost.

| Missing capability | Effect |
|---|---|
| message id | synthesized from timestamp, nick, and content, and marked as synthetic |
| account tag | identity resolved by nick; `Attribution: nick` |
| server time | receipt time is used |
| chat history | backfill reports unsupported; the gap is noted and nothing else is done |

The bot's own messages are skipped, so wsm never records its own echo.

### Cursors and backfill

A per-channel cursor records the last message id and time. Backfill pages history forward from the cursor until a short page, skipping the cursor's own message and anything already present in the log; the recorder is the deduplicating authority, so a crash between recording and cursor write costs a duplicate check, not a duplicate event.

A gap is decided by retention, never by a time heuristic: the oldest retained message is fetched first, and only if it is newer than the cursor did the window roll past. A quiet channel is not a gap.

Backfill never opens a second session beside the daemon. With the daemon running, `wsm irc backfill` files a request the daemon executes and reports through its status file; with the daemon stopped, it runs under the same exclusive state lock the daemon holds while alive. Cursors are written only by the lock holder.

## Acceptance Scenarios

1. **Given** a member speaks in a template channel, **When** the bot receives it, **Then** one `interaction.message` event is recorded carrying the channel, nick, message id, and attribution.
2. **Given** a message tagged with an account, **When** it is recorded, **Then** the agent id is the profile resolved from that account and `Attribution` is `account`.
3. **Given** a server that does not send account tags, **When** a message is recorded, **Then** identity is resolved by nick and `Attribution` is `nick`.
4. **Given** a server that does not send message ids, **When** a message is recorded, **Then** a synthetic id is derived from timestamp, nick, and content and is marked as synthetic.
5. **Given** a server that does not send timestamps, **When** a message is recorded, **Then** the receipt time is used and the event is still recorded.
6. **Given** the bot's own message, **When** it is echoed back, **Then** nothing is recorded.
7. **Given** a recorded message, **When** the cursor is written, **Then** it advances per message; a recorder error leaves the cursor unchanged so the message is retried rather than lost.
8. **Given** a daemon restart with prior history, **When** backfill runs, **Then** messages after the cursor are recorded in order, and a second run records nothing new.
9. **Given** history pages that overlap, **When** backfill runs, **Then** each message is recorded once.
10. **Given** a channel whose oldest retained message is newer than the cursor, **When** backfill runs, **Then** a gap is reported; **and given** a merely quiet channel, **Then** no gap is reported.
11. **Given** a server without chat history, **When** backfill runs, **Then** it reports unsupported, issues no history commands, and exits 0.
12. **Given** the daemon is running, **When** I run `wsm irc backfill`, **Then** the daemon executes it and reports through status rather than a second connection being opened.
13. **Given** the daemon is stopped, **When** I run `wsm irc backfill`, **Then** it runs under the exclusive state lock.

## Tests

### Unit
- `pkg/irc/bot/capture_test.go` -- `TestCaptureAttribution`
- `pkg/irc/bot/capture_test.go` -- `TestCaptureSkips`
- `pkg/irc/bot/capture_test.go` -- `TestCaptureNoticeIsCaptured`
- `pkg/irc/bot/capture_test.go` -- `TestCaptureServerTime`
- `pkg/irc/bot/capture_test.go` -- `TestCaptureMsgIDDegradation`
- `pkg/irc/bot/capture_test.go` -- `TestCaptureTagOrderIsIrrelevant`
- `pkg/irc/bot/capture_test.go` -- `TestCaptureFromHistoryFixture`
- `pkg/irc/bot/capture_test.go` -- `TestCaptureRunSubscribesToChannelTraffic`
- `pkg/irc/bot/cursor_test.go` -- `TestCaptureAdvancesCursor`
- `pkg/irc/bot/cursor_test.go` -- `TestCaptureRecorderErrorLeavesCursorUnchanged`
- `pkg/irc/bot/cursor_test.go` -- `TestCaptureSkippedMessageLeavesCursorUnchanged`
- `pkg/irc/bot/cursor_test.go` -- `TestCursorsRoundTrip`
- `pkg/irc/bot/cursor_test.go` -- `TestCursorsSaveIsAtomic`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillPagesUntilShortPage`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillGapDetection`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillGapProbeUsesEpochSelector`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillEmptyChannel`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillUnsupportedIssuesNoCommands`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillSkipsCursorMsgID`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillDedupesAgainstSeen`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillOverlappingPageRecordsOnce`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillAdvancesCursorPerMessage`
- `pkg/irc/bot/backfill_test.go` -- `TestBackfillRecorderErrorStopsRun`
- `pkg/irc/bot/caps_test.go` -- `TestDefaultCapsCoverTheDesign`
- `pkg/irc/daemon/recorder_test.go` -- `TestRecorderSetsWriterIDBeforeAlign`
- `pkg/irc/daemon/recorder_test.go` -- `TestRecorderPreservesTimestampAndAgent`
- `pkg/irc/daemon/recorder_test.go` -- `TestSeenFindsNewestBeyondOnePage`
- `pkg/irc/daemon/recorder_test.go` -- `TestSeenStopsAtSince`
- `pkg/irc/daemon/daemon_test.go` -- `TestReconnectTriggersBackfill`
- `pkg/irc/daemon/daemon_test.go` -- `TestBackfillRequestsCoalesce`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCBackfill_NormalizeChannels`
- `cmd/wsm/commands/irc_extras_test.go` -- `TestIRCBackfill_RequestIsAtomicAndReadable`

### E2E
- `tests/e2e/irc_extras_test.go` -- `TestIRCExtras_BackfillViaDaemon`
- `tests/e2e/irc_extras_test.go` -- `TestIRCExtras_BackfillOfflineHoldsTheLock`
