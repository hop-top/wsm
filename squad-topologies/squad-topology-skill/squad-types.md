# Squad Types Reference

## 1. Stream-Aligned Squad

**Purpose:** The primary value-producing unit. Aligned to a continuous flow of
work in a specific domain.

**Criteria:**
- Owns its pipeline end-to-end: intake → processing → output
- A single agent within the squad can complete a meaningful unit of work without
  calling out to another squad
- Context boundary is the domain: the squad holds all knowledge needed to act

**Plain language analogy:** A department that handles one type of work from start
to finish. Like a sales team that handles the full cycle from lead to close.

**Key question for identification:**
> "If we cut this squad off from all other squads, does it degrade gracefully
> or collapse immediately?"

**Signs a squad is stream-aligned:**
- Has a clear trigger (input) and a clear deliverable (output)
- Runs repeatedly (daily, weekly, on-demand) against the same type of work
- Could be described as "the [X] squad" — e.g., "the lead research squad"

**Warning signs:**
- Squad regularly coordinates with 3+ other squads to complete a single task
- Squad is doing two clearly different types of work
- No clear owner: "who is responsible for the output of this squad?"

---

## 2. Enabling Squad

**Purpose:** Temporarily closes a capability gap in a stream squad, then exits.

**Criteria:**
- No permanent domain ownership
- Must have a defined exit condition before attachment begins
- Success = leaving a capability behind and making itself unnecessary

**Plain language analogy:** A consultant or trainer brought in to teach a team
a new skill, then leaves.

**Key question for identification:**
> "What specific capability is being embedded, and when exactly is this squad done?"

**Signs a squad is enabling:**
- It's teaching, not producing
- There's a natural end point ("once the lead research squad can do X on its own")
- Its value decreases over time as the stream squad absorbs the capability

**Warning signs:**
- Enabling squad has been attached for months with no exit plan
- Stream squad is still dependent on enabling squad for routine work
- No handoff artifact (documentation, tooling, embedded prompt) was ever produced

---

## 3. Complicated Subsystem Squad

**Purpose:** Wraps a hard, specialized problem behind a clean interface.

**Criteria:**
- Hard problem that would be prohibitively complex for a stream squad to handle
- Narrow, stable interface (inputs/outputs)
- Internal implementation is opaque to consumers

**Plain language analogy:** A specialist team (legal, finance, engineering) that
other teams call on for specific answers — but the other teams don't need to
understand how the specialist arrives at those answers.

**Key question for identification:**
> "Is the interface narrow enough that a stream squad never needs to understand
> what happens inside?"

**Signs a squad is a complicated subsystem:**
- The domain requires specialized knowledge most squads don't have
- Multiple squads are trying to replicate the same logic → extract it
- The logic is stable and well-defined, even if complex

**Warning signs:**
- Interface keeps changing, forcing cascade updates in consuming squads
- Consumers are reaching into implementation details to work around the interface
- Nobody can explain what the subsystem does in one sentence

---

## 4. Platform Squad

**Purpose:** Builds and maintains the shared substrate that all other squads consume.

**Criteria:**
- Self-service by design — consuming squads never need to file a ticket
- Versioned contracts — breaking changes communicated, not discovered
- Measures its health by the friction it removes from stream squads

**What platform squads typically provide:**
- Tool registries (which tools are available, with versioned schemas)
- Shared memory primitives (session state, cross-squad knowledge)
- Prompt versioning and management
- Cost and latency telemetry per squad
- Golden path for onboarding new squads

**Plain language analogy:** The IT department or internal tooling team — everyone
uses their infrastructure, but nobody thinks about how it works.

**Key question for identification:**
> "Can a new stream squad reach production-ready state using only the platform's
> golden path, without help from a platform team member?"

**Warning signs:**
- Stream squads are reaching into platform internals instead of consuming the API
- Platform squad is on the critical path of any stream squad
- No telemetry: nobody knows what the platform costs per squad to run

---

## 5. Orchestrator Squad

**Purpose:** Decomposes goals, routes tasks, and synthesizes results across squads.

**Criteria:**
- No domain ownership of its own
- Permanently declared — not emergent from implementation
- Owns the routing and synthesis logic, not the work itself

**Plain language analogy:** A project manager or dispatcher — they don't do the
work, but they make sure the right people get the right work at the right time,
and they assemble the final result.

**Key question for identification:**
> "Is there a coordination role that's currently being handled implicitly
> (by a stream squad that's overloaded, or by nobody at all)?"

**Signs an orchestrator is needed:**
- Complex multi-squad workflows with conditional routing
- A stream squad is spending significant time "figuring out what to do next"
  rather than doing its core work
- Results from multiple squads need to be synthesized before delivery

**Warning signs:**
- Orchestrator squad has become a bottleneck (delegation contracts too tight)
- Orchestrator is doing actual domain work instead of routing
- "Orchestration" is happening inside a stream squad that was meant for something else

---

## 6. Sentinel Squad

**Purpose:** Continuous observation — monitors all squads for drift, quality
degradation, cost overruns, and safety boundary violations.

**Criteria:**
- Reactive, not proactive: observes and alerts, doesn't intervene directly
- Must have a reclaim path: detecting without ability to correct is just logging
- Protects the conditions under which other squads produce value

**Plain language analogy:** Quality assurance, compliance, or an internal auditor
— they don't do the work, but they make sure the work stays within bounds.

**Key question for identification:**
> "Who is responsible for noticing when something is going wrong across the
> entire system — before it cascades?"

**Signs a sentinel is needed:**
- High-stakes outputs where drift or errors have real consequences
- No existing mechanism for catching systematic degradation
- Cost overruns have happened before and weren't caught early

**Warning signs:**
- Sentinel has no reclaim path (detection without correction authority = just logging)
- Sentinel is generating alerts nobody acts on
- Sentinel scope is too broad (monitoring everything → noticing nothing)
