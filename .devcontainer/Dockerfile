FROM ubuntu:24.04

ARG DEBIAN_FRONTEND=noninteractive
# Keep in step with the go.mod directive; Dockerfile.ubuntu-test pins the
# same toolchain for the CI matrix.
ARG GO_VERSION=1.26.1
ARG ERGO_VERSION=2.19.1
ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
  bash-completion \
  build-essential \
  ca-certificates \
  curl \
  expect \
  git \
  make \
  pkg-config \
  util-linux \
  && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" -o /tmp/go.tgz \
  && rm -rf /usr/local/go \
  && tar -C /usr/local -xzf /tmp/go.tgz \
  && rm -f /tmp/go.tgz

# ergo ships prebuilt tarballs; the release manifest publishes a sha256 per
# asset, so pin both arches rather than trusting the download alone. The
# archive nests everything under ergo-<ver>-linux-<arch>/, hence strip-1.
RUN set -eux; \
  case "${TARGETARCH}" in \
    arm64) ergo_arch='arm64'; ergo_sha='3f68fee0ede09eeeb205a553fe9727b409c686752e1cc2fea34ae2f9d64517fb' ;; \
    amd64) ergo_arch='x86_64'; ergo_sha='c275f2de8e8eb074c38707136392290172400b5b78616cb9f660351527924b47' ;; \
    *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
  esac; \
  curl -fsSL -o /tmp/ergo.tgz \
    "https://github.com/ergochat/ergo/releases/download/v${ERGO_VERSION}/ergo-${ERGO_VERSION}-linux-${ergo_arch}.tar.gz"; \
  echo "${ergo_sha}  /tmp/ergo.tgz" | sha256sum -c -; \
  mkdir -p /opt/ergo; \
  tar -C /opt/ergo --strip-components=1 -xzf /tmp/ergo.tgz; \
  rm -f /tmp/ergo.tgz; \
  ln -sf /opt/ergo/ergo /usr/local/bin/ergo

ENV PATH="/usr/local/go/bin:${PATH}" \
  CGO_ENABLED=1 \
  GOCACHE=/workspace/.tmp/gocache \
  ERGO_BIN=/usr/local/bin/ergo \
  ERGO_DEFAULT_CONFIG=/opt/ergo/default.yaml

WORKDIR /workspace
