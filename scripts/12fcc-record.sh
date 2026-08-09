#!/usr/bin/env bash
# Record 12fcc conformance cassettes from real wsm runs.
#
# For every scenario under e2e/conformance/scenarios/wsm/, drives
# `kit conformance harness record` against a freshly built binary:
# each step executes as a real subprocess; exit code, stdout, stderr,
# and duration are captured verbatim into the svc upload layout the
# grading service consumes:
#
#   e2e/conformance/cassettes/<scenario-id>/
#     manifest.yaml
#     story.yaml
#     steps/<step-id>/{result.json,stdout.txt,stderr.txt}
#
# Captures are never edited after the fact; re-running re-records
# everything from scratch. The recorder refuses to proceed when the
# story bytes do not hash to the scenario's declared content_hash.
#
# Isolation: every scenario gets its own work dir under a pinned root.
# XDG_* point inside it and WSM_* env vars are scrubbed, so a scenario
# can only see the state it creates itself — never the developer's
# real workspace DB. The data dir is passed via WSM_DATA_DIR rather
# than a --data-dir flag so the recorded invocations stay byte-identical
# to what the story documents.
#
# Host-identity scrub: `wsm status` echoes the resolved user and the
# KIT_* env surface, so an unscrubbed recording bakes the operator's
# username and this script's own KIT_BIN path into committed
# captures — machine-specific noise that churns on every re-record and
# leaks local paths into the repo. USER/LOGNAME are pinned to a
# fixed placeholder and KIT_BIN is dropped from the child env (the
# recorder is invoked by absolute path, so it needs no such variable).
#
# One field resists scrubbing: kit's profile provider prefers
# user.Current(), which reads the OS passwd record and ignores USER.
# The recording therefore still carries the recorder's username in
# sections[profile].data.user. The status-json rubric asserts that
# field is *present* rather than equal to a value, so the verdict
# stays host-independent; only the captured byte differs per machine.
#
# Requirements:
#   - KIT_BIN: path to a kit binary that ships `conformance harness
#     record` (build from hop.top/kit cmd/kit). Defaults to `kit` on
#     PATH.
#
# Usage: [KIT_BIN=/path/to/kit] scripts/12fcc-record.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONF="$REPO_ROOT/e2e/conformance"
BIN="$REPO_ROOT/bin/wsm"
KIT_BIN="${KIT_BIN:-kit}"
WORK="${WSM_12FCC_WORK:-/tmp/wsm-12fcc-record}"

# The probe requires real help output (not just exit 0) so a wrong
# binary can never masquerade as a conformance-capable kit.
probe="$("$KIT_BIN" conformance harness record --help 2>/dev/null || true)"
case "$probe" in
  *--scenario*) ;;
  *)
    echo "ERROR: '$KIT_BIN' does not ship 'conformance harness record'. Set KIT_BIN to a kit binary built from hop.top/kit cmd/kit." >&2
    exit 1
    ;;
esac

echo "==> building $BIN"
(cd "$REPO_ROOT" && go build -buildvcs=false -o "$BIN" ./cmd/wsm)
BINARY_VERSION="$(cd "$REPO_ROOT" && git rev-parse --short HEAD 2>/dev/null || echo dev)"

# Scrub ambient wsm config sources: recorded behavior must derive from
# the scenario's own writes plus built-in defaults only.
while IFS='=' read -r name _; do
  case "$name" in WSM_*) unset "$name" ;; esac
done < <(env)

rm -rf "$WORK"
mkdir -p "$WORK"

for scenario in "$CONF"/scenarios/wsm/*/*/scenario.yaml; do
  sid="$(basename "$(dirname "$(dirname "$scenario")")")"
  echo "==> recording $sid"
  proj="$WORK/$sid"
  mkdir -p "$proj/xdg" "$proj/data" "$proj/home"
  rm -rf "${CONF:?}/cassettes/$sid"
  env -u KIT_BIN \
    HOME="$proj/home" \
    USER=conformance \
    LOGNAME=conformance \
    XDG_CONFIG_HOME="$proj/xdg" \
    XDG_DATA_HOME="$proj/xdg-data" \
    XDG_STATE_HOME="$proj/xdg-state" \
    XDG_CACHE_HOME="$proj/xdg-cache" \
    WSM_DATA_DIR="$proj/data" \
    NO_COLOR=1 \
    "$KIT_BIN" conformance harness record \
      --scenario "$scenario" \
      --binary "$BIN" \
      --binary-version "$BINARY_VERSION" \
      --out "$CONF/cassettes/$sid" \
      --workdir "$proj" \
      --no-hints >/dev/null
done

echo "==> cassettes written to $CONF/cassettes"
