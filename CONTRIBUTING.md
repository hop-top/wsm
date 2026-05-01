# Contributing

Thanks for contributing to `hop.top/wsm`.

## Development Workflow

1. Read `README.md` and `AGENTS.md` first.
2. Confirm target stories in `docs/stories/README.md`.
3. Implement changes against the corresponding plan file in `docs/plans/`.
4. Add or update tests before opening a pull request.

## Local Checks

```bash
make test
make verify-no-ui-leak
```

## Pull Requests

1. Keep scope aligned to one plan segment where possible.
2. Explain story IDs covered in PR description.
3. Update docs when behavior or interfaces change.
