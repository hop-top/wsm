# Dev container

Ubuntu 24.04 with the toolchain the repo's gates assume: Go matching the
`go.mod` directive, a C toolchain (`mattn/go-sqlite3` needs cgo), and a
pinned `ergo` IRC server for the workspace-chat feature.

## What's inside

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.26.1 | from go.dev, same source as `Dockerfile.ubuntu-test` |
| ergo | 2.19.1 | prebuilt release tarball, sha256-pinned per arch |
| build-essential, pkg-config | — | `CGO_ENABLED=1` for sqlite3 |
| expect, util-linux | — | interactive-completion E2E tests |

Both arches are pinned: `arm64` and `amd64` resolve to the matching release
asset and its published checksum. An unsupported `TARGETARCH` fails the
build rather than installing something unverified.

## ergo

```
ergo --version                  # 2.19.1
$ERGO_BIN                       # /usr/local/bin/ergo
$ERGO_DEFAULT_CONFIG            # /opt/ergo/default.yaml — base for probes
```

`default.yaml` is the upstream shipped config; copy it into a scratch dir
and edit there. Never point ergo at the file in `/opt/ergo` directly.

Typical loop for probing ergo surfaces:

```sh
mkdir -p /tmp/ergo-probe && cd /tmp/ergo-probe
cp "$ERGO_DEFAULT_CONFIG" ergo.yaml
# languages.path ships relative ("languages"), so it only resolves with
# cwd inside the unpacked release. Point it at the install before anything
# reads the config, or every subcommand dies on "Could not load languages".
sed -i 's|^\( *path: \)languages$|\1/opt/ergo/languages|' ergo.yaml
# edit ergo.yaml further, then, in this order — the config names the cert
# files relatively, so initdb fails if mkcerts has not run in this dir:
ergo mkcerts --conf ergo.yaml   # writes fullchain.pem + privkey.pem
ergo initdb  --conf ergo.yaml   # writes ircd.db
ergo run     --conf ergo.yaml
```

Verified in this image: the server comes up on `:6697` (TLS) and
`127.0.0.1:6667` (plaintext) from a scratch dir with the sequence above.

The integration suite keys off `ERGO_BIN` and skips when it is unset, so
the tagged tests run here and stay skipped on machines without ergo.

## Gates

```sh
make build              # binary + all packages
make test               # unit + default E2E
make test-e2e           # default E2E only
make verify-no-ui-leak  # pkg/* must not pull in charmbracelet
```

`make test-ubuntu-docker` builds its own image from `Dockerfile.ubuntu-test`
and is redundant inside the dev container.
