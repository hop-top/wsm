# projects

rux project registry; reader/writer of `~/.config/rux/projects.yaml`.

- writes: `wsm space add` (and `wsm space sync`)
- reads: `rux connect <name>`
- locks: `gofrs/flock` on sidecar `projects.yaml.lock`
