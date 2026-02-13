# Personas

Actor personas for the wsm (Workspace & Session Manager) project.

| Persona | Description | Discovery | Delivery | Stories |
|---------|-------------|-----------|----------|---------| 
| [User](user.md) | End-user; CLI/TUI workflows | Web manual, `--help`, TUI hints, man pages | Web guides, plain text, prompts, color output | 14 |
| [Maintainer](maintainer.md) | Tests, conformance, governance | GoDoc, CONTRIBUTING, CI logs, git history | Markdown, test reports, diffs | 5 |
| [External Client](external-client.md) | Automation / scripted consumers | API docs, JSON schemas, exit-code table | JSON, exit codes, versioned schemas | 4 |
| [Integrator](integrator.md) | Embedding `pkg/` as Go module | GoDoc, examples/, interface contracts | Go source, example tests, semver notes | 5 |
