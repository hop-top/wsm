# Anti-Patterns Reference

Common design mistakes and how to spot and fix them.

---

## Anti-Pattern 1: The Sprawling Squad

**What it looks like:** A single squad that owns multiple value streams, or
whose "briefing packet" would be enormous.

**Symptom:** "The squad does X, Y, Z, and also handles A when needed."

**Why it's bad:** Context load explosion. Agents in the squad can't hold enough
domain knowledge to do any one thing well. Everything becomes mediocre.

**Fix:** Identify the natural fracture plane. Usually it's the domain boundary:
"lead research" vs "proposal writing" vs "client onboarding" are three squads,
not one.

**Diagnostic question:** "Could you describe this squad's job in one sentence,
without using 'and' more than once?"

---

## Anti-Pattern 2: The Eternal Collaboration

**What it looks like:** Two squads that have been "collaborating" for months
with no end in sight.

**Symptom:** "Squad A and Squad B work together on this." When did they start?
"A while ago." When will they finish? "It's ongoing."

**Why it's bad:** Context entanglement. Neither squad can operate independently.
The collaboration has become a hidden dependency.

**Fix:** Either graduate to X-as-a-Service (define the interface, freeze it,
separate the squads), or merge the squads if they genuinely can't be separated.

**Diagnostic question:** "What would Squad A deliver to Squad B if they had to
put it in a document and hand it over, with no further conversation?"

---

## Anti-Pattern 3: The Homeless Orchestrator

**What it looks like:** Multi-squad coordination happening inside a stream squad
that was meant for something else.

**Symptom:** "The lead generation squad figures out which squad to call next
based on what it finds."

**Why it's bad:** The stream squad is carrying extraneous coordination load.
Its context window is consumed by routing logic instead of domain knowledge.

**Fix:** Extract the coordination logic into an explicit Orchestrator squad.
Let the stream squad do only its domain work.

**Diagnostic question:** "Is this squad spending significant effort deciding
what to do next, or does it just do its one thing?"

---

## Anti-Pattern 4: The Invisible Platform

**What it looks like:** Every squad has its own version of shared infrastructure —
tool registry, memory layer, prompt templates — built in isolation.

**Symptom:** "Each squad manages its own tools." Or: no one can answer "where
is the canonical tool registry?"

**Why it's bad:** Duplication of effort, inconsistent behavior, no central
observability, and massive overhead when anything changes.

**Fix:** Identify what's truly shared (tool registry, memory primitives, cost
telemetry) and extract it into a Platform squad. Even a minimal platform is
better than N independent versions.

**Diagnostic question:** "Is there any infrastructure that more than two squads
rely on? Where does it live?"

---

## Anti-Pattern 5: The Replicated Subsystem

**What it looks like:** Multiple squads each implementing their own version of
a complex, specialized capability.

**Symptom:** "Both the lead scoring squad and the proposal squad have their own
version of the ICP matching logic."

**Why it's bad:** Inconsistency (two squads will drift apart), wasted effort,
and no single place to improve or audit the logic.

**Fix:** Extract the specialized logic into a Complicated Subsystem squad with
a clean interface. Both squads consume it via X-as-a-Service.

**Diagnostic question:** "Is there any complex logic that more than one squad
is implementing independently?"

---

## Anti-Pattern 6: The Exitless Enabler

**What it looks like:** An enabling squad with no exit condition, attached
indefinitely to a stream squad.

**Symptom:** "The [capability] squad helps the [stream] squad with [X]." Is
this temporary? "Yeah, but we haven't set an end date."

**Why it's bad:** The stream squad never becomes self-sufficient. The enabling
squad gets trapped. The capability never gets properly embedded.

**Fix:** Define the exit condition NOW, even retroactively. "Self-sufficient
means the stream squad can do X without contacting the enabling squad for 4
consecutive weeks."

**Diagnostic question:** "What would the enabling squad's last day look like?
What would have been accomplished?"

---

## Anti-Pattern 7: The Blind Sentinel

**What it looks like:** A sentinel squad that detects problems but has no
authority or mechanism to do anything about them.

**Symptom:** "The sentinel generates alerts, but they go into a queue that
nobody checks." Or: "We know drift is happening but we can't stop the squad
from running."

**Why it's bad:** Detection without correction is just expensive logging.
The value of a sentinel is the ability to course-correct, not just observe.

**Fix:** Define the reclaim path. What happens when the sentinel detects a
problem? Who or what gets notified? What can be paused, redirected, or flagged?

**Diagnostic question:** "If the sentinel detects a squad is drifting at 2am,
what happens next, concretely?"

---

## Anti-Pattern 8: The Topology-After-Architecture Mistake

**What it looks like:** The squad design was reverse-engineered from an
existing agent implementation, not designed intentionally first.

**Symptom:** "We built it and then realized we needed to describe how it's
organized." The squad names match function names. The interaction modes match
function calls.

**Why it's bad:** The topology reflects implementation convenience, not business
value flow. Squads are too granular, too coupled, or organized around technical
layers instead of domains.

**Fix:** Use the existing implementation as input to a proper topology design
session (this skill). Map what exists to the 6 squad types. Identify what should
be refactored.

**Diagnostic question:** "Does this squad topology make sense to a business
owner who has never seen the code?"
