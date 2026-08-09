# Conformance grading (12fcc service tier)

Scenario library, recorded cassettes, and captured verdicts for the
service-graded factors of the 12-Factor AI-CLI conformance contract:
F3 Structured I/O, F4 Corrective Error Model, F5 Explicit Contracts,
F6 Previewability, F7 Idempotency, F8 State Transparency, and F11
Exit Code Semantics. The story-graded factors (F1, F2, F9, F12) and
leak scanning (F10) are covered by the `12fcc` workflow over
`e2e/stories/`.

## Layout

```
scenarios/wsm/<id>/1.0.0/scenario.yaml    graded rubric, one per scenario
stories/<id>.yaml                         user story each scenario is bound to
cassettes/<id>/                           captures from real binary runs
  manifest.yaml                           upload manifest (story hash, steps)
  story.yaml                              byte-exact copy of the story
  steps/<step-id>/result.json             exit code + duration as observed
  steps/<step-id>/stdout.txt              stdout as observed
  steps/<step-id>/stderr.txt              stderr as observed
verdicts/<id>.json                        grading service verdict, tier 3
```

Cassettes are recorded, never authored: every stdout/stderr byte and
exit code comes from executing the actual binary via `kit conformance
harness record`. Scenarios encode the spec-correct expectation even
where current behavior violates it — a failing verdict is the honest
outcome, not a broken pipeline. Re-records churn only `recorded_at`,
`binary_version`, and `duration_ms`; captures are byte-stable.

Each scenario runs in its own throwaway work dir with `HOME` and the
`XDG_*` roots pointed inside it, `WSM_DATA_DIR` pinned to a scenario-
local SQLite store, and ambient `WSM_*` env scrubbed — so a scenario
sees only the state it creates itself, never the developer's real
workspace database.

## Re-running

Record fresh cassettes (builds the binary, isolates a work dir per
scenario, runs every scenario step as a real subprocess):

```sh
KIT_BIN=/path/to/kit make 12fcc-record
```

Grade them against a locally served scenario library (requires a
`kit` binary that ships the `conformance` command group; point
`KIT_BIN` at one built from `hop.top/kit` `cmd/kit`):

```sh
KIT_BIN=/path/to/kit make 12fcc-grade
```

The grade target boots `kit conformance svc serve` on a loopback port
with `--scenarios-root e2e/conformance`, mints a `grade:wsm` token
into a throwaway claims DB, uploads every cassette at tier 3,
rewrites `verdicts/`, and prints per-scenario verdicts plus a
per-factor rollup. Grading is measurement, not a gate: the target
only fails when a cassette cannot be graded at all. Set `STRICT=1` to
also fail on `fail` verdicts.

## Badge

`.12fc.json` at the repo root — the shields.io endpoint behind the
README badge — is committed and is the source of truth for all 12
factors. CI never rewrites it: the `12fcc` workflow can only run the
story and leak leaves (F1, F2, F9, F10, F12), and the badge verdict
counts measured factors only, so a CI refresh would degrade the
truthful `12/12 pass` badge to a red `5/5 pass` one. The workflow
keeps its badge commit disabled until a released `kit` can genuinely
grade cassettes in CI; the enable checklist lives in
`.github/workflows/12fcc.yaml`.

Refresh the badge locally, from measurements only:

```sh
KIT_BIN=/path/to/kit make 12fcc-record   # re-record cassettes from the real binary
KIT_BIN=/path/to/kit make 12fcc-grade    # tier-3 verdicts into verdicts/
KIT_BIN=/path/to/kit make 12fcc-badge    # verify leaves + verdicts -> .12fc.json
```

Commit the resulting `.12fc.json` diff together with the re-recorded
cassettes and verdicts.

## Current verdicts

As of the captures in `cassettes/` (see `verdicts/` for full facets):

| Scenario | Verdict | Factors |
|---|---|---|
| `status-json` | pass | F3 F8 F11 |
| `capabilities-contract` | pass | F5 F11 |
| `error-envelope-correction` | pass | F4 F11 |
| `workspace-add-preview` | pass | F3 F6 F11 |
| `workspace-archive-idempotent` | pass | F7 F11 |
| `workspace-state-transparency` | pass | F3 F8 F11 |

Factor rollup: every graded factor (F3, F4, F5, F6, F7, F8, F11)
passes. The behaviors that carry the verdicts:

- **F3** — `status`, `workspace show`, `workspace list`, and the
  dry-run plans render one JSON document on stdout under
  `--format json`; diagnostics stay on stderr.
- **F4** — error envelopes carry `code` + `suggested_fix`: a missing
  workspace echoes the failing reference and names the listing
  command that reveals valid ones; unknown flags, unknown commands,
  and unknown subcommands under a real group all point at the
  failing node's own `--help`.
- **F5** — `wsm capabilities` serves the machine-readable command
  surface built on kit's toolspec walker (side-effect and idempotency
  contracts per leaf); `--api-version` pins are honored; formats
  outside the declared registry are refused loudly.
- **F6** — `--dry-run` on a mutating leaf returns a structured plan
  naming the target and the declared effects, and applies nothing —
  the follow-up listing stays empty.
- **F7** — repeat reads converge: reading the same workspace twice
  reports identical status and counts, and the listing still holds
  exactly one entry.
- **F8** — the `wsm` status section reports the *resolved* backend,
  data dir, and winning config layer (kit's `effective-config`
  section shows raw flag values, which read empty when unset);
  `workspace show` exposes status, event and snapshot counts, and the
  event log as typed fields.
- **F11** — the process honors envelope-declared exit codes and the
  kit exit-class table (USAGE=2, NOT_FOUND=3, CONFLICT=4); unknown
  commands are usage errors instead of help + exit 0.

If a regression re-introduces a defect, re-record and re-grade; the
scenario library encodes the spec-correct expectation, so the verdict
flips without rubric changes.
