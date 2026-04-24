# Identity Scaffold Format
## Output format for agent identity provisioning

Generated after triage + topology design. Produces the files
needed to instantiate agent profiles with proper identity,
facets, secrets, and workspace bindings.

---

## Inputs

The scaffold consumes structured data from the triage:

```yaml
identity:
  platforms: [github, email, slack]    # Q6
  delegation: false                     # Q7
  delegates: []                         # Q7 detail
  contexts: 1                           # Q8
  context_names: [default]              # Q8 detail
profiles:                               # from Steps 3-5
  - id: agent-name
    type: assistant                     # profile archetype
    tier: 0                             # 0 or 1
    squad: null                         # squad name if tier 1
```

---

## Output Files

### 1. Profile — `profiles/<id>/profile.yaml`

```yaml
id: <agent-id>
name: <display name>
email: <primary email>

character:
  traits:
    - <trait-1>                         # e.g., methodical
    - <trait-2>                         # e.g., concise
  communication_style: <style>          # e.g., direct, warm
  dissent_tendency: <low|medium|high>   # profile-level invariant
  expertise:
    - <domain-1>
    - <domain-2>

avatar: avatar.png                      # relative path, stub

# Optional — set by Squad Topologies for Tier 1
squad: <squad-name>
profile_type: <0-6>                     # agent profile archetype
```

---

### 2. Facet — `profiles/<id>/facets/<context>.yaml`

One file per operational context. Single-context agents get
`default.yaml`.

```yaml
name: <context-name>
display_name: <context-specific display name>
email: <context-specific email>
avatar: <context-specific avatar, optional>

handles:
  github: <username>                    # present if github in Q6
  x: <handle>                          # present if x in Q6
  slack: <member-id>                   # present if slack in Q6
  linkedin: <profile-url>             # present if linkedin in Q6
  # additional handles per Q6 platforms

git:
  user_name: <git display name>
  user_email: <git email>
  signing_key: <key-id>                # stub — populated by secrets
  gpg: true                            # or false

tone:
  register: <formal|casual|technical>
  notes: <context-specific tone guidance>

governance:
  - <rule-1>                           # e.g., "never send without approval"
  - <rule-2>                           # e.g., "always sign commits"

ctxt:
  default_lens: <lens-name>            # scoped knowledge context
```

---

### 3. Secrets Config — `profiles/<id>/secrets.yaml`

No actual secrets. Schema + adapter config only.

```yaml
adapter: age                            # age | env | sops | 1password
config:
  identity_file: ~/.config/age/keys.txt # adapter-specific config
  recipients_file: .age-recipients

scopes:
  profile:                              # profile-level secrets
    - name: signing_key
      description: GPG/age signing key
    - name: api_token
      description: Primary API token

  facets:
    <context-name>:                     # per-facet secrets
      - name: smtp_password
        description: Email credentials
      - name: github_token
        description: GitHub PAT
      - name: slack_token
        description: Slack bot token
      # ... per platform from Q6
```

---

### 4. Workspace Config Snippet

Emitted as a block the user pastes into their wsm workspace
config. One member block per agent.

```yaml
members:
  - profile: <agent-id>
    facet: <context-name>               # from Q8
    capacity: principal                 # or delegate (from Q7)
    # delegate fields — only when capacity=delegate
    delegate_for: <entity-name>
    audit_label: "<agent-id> acting as <entity>"
    # secrets scope
    secrets_scope: facets/<context-name>
```

For multi-facet agents, emit one member block per facet-context
pair. The agent switches facets based on which workspace/space
is active.

---

### 5. README — `profiles/<id>/README.md`

```markdown
# <Agent Name>

<One-line purpose from the Agent Brief or Squad Registry.>

## Technical

- [Architecture](docs/architecture.md)
- [Data flow diagram](docs/data-flow-v1.mmd)
- [xray map](docs/xray-map.md)

## Contributing

- [Adopted patterns](docs/patterns.md)
- [ADRs](docs/adr/)
- [Stories](docs/stories/)
- [Future plans](docs/roadmap.md)

## User Manual

- [Installation & setup](docs/setup.md)
- [Quick wins](docs/quick-wins.md)
- [Example usage](docs/examples.md)
- [E2E test references](docs/e2e-tests.md)
- [Troubleshooting](docs/troubleshooting.md)
```

Stub all links. Implementation fills them in progressively.

---

## Scaffold File Tree

Complete scaffold for a single-context agent:

```
profiles/<id>/
  profile.yaml
  facets/
    default.yaml
  secrets.yaml
  avatar.png              # placeholder
  README.md
```

Multi-context agent (3 contexts):

```
profiles/<id>/
  profile.yaml
  facets/
    oss.yaml
    internal.yaml
    client.yaml
  secrets.yaml
  avatar.png
  README.md
```

---

## Tier-Specific Behavior

### Tier 0 (single agent)

- One profile, one facet (usually `default`)
- Secrets config with profile-level scope only (unless
  multi-context from Q8)
- Workspace snippet is optional — user may not use wsm
- README links to Agent Brief as primary doc

### Tier 1 (multi-squad)

- One profile per squad member agent
- Orchestrator agents: check Q7 — if they dispatch work as
  other agents, set capacity=delegate
- Platform squad agents: broader handle coverage (more
  platforms in facets)
- Sentinel agents: minimal facets (often internal-only)
- Swarm units: template profile — one definition, N instances

---

## Presentation

Present the scaffold as a file tree with key values filled in.
Example:

```
profiles/
  email-drafter/
    profile.yaml          # name: Email Drafter, type: assistant
    facets/
      default.yaml        # email: bot@company.com, tone: warm
    secrets.yaml           # adapter: age, scopes: [smtp_password]
    README.md
```

Ask: "Does this structure look right? I can adjust profiles,
facets, or platform handles before finalizing."

After confirmation, emit the full YAML content for each file.
