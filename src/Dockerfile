# syntax=docker/dockerfile:1
#
# Hermetic build: every embedded frontend asset (the octadground board bundle,
# its base stylesheet, and the Tailwind app stylesheet) is fetched from a pinned
# source and (re)built inside the image, then embedded via //go:embed static/* by
# the Go build. Nothing frontend-related is committed to this repo — see
# .gitignore and CLAUDE.md. This makes `fly deploy` (and any CI image build)
# self-contained: no sibling repos, no local Tailwind binary, no manual copying.

# ---- octadground: fetch the pinned source and build the minified board bundle
FROM node:22-bookworm-slim AS assets-octadground
# renovate: datasource=github-tags depName=dechristopher/octadground
ARG OCTADGROUND_REF=v7.12.2
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /og
# --frozen-lockfile pins the build toolchain (rollup/typescript/terser) via the
# repo's yarn.lock; `yarn dist` emits dist/octadground.min.js (terser IIFE).
RUN git clone --depth 1 --branch "${OCTADGROUND_REF}" \
      https://github.com/dechristopher/octadground.git . \
    && yarn install --frozen-lockfile \
    && yarn dist

# ---- web assets: minified Tailwind CSS (standalone CLI) + minified JS (esbuild),
#      both from pinned single binaries — no npm client involved
FROM debian:bookworm-slim AS assets-web
# renovate: datasource=github-releases depName=tailwindlabs/tailwindcss
ARG TAILWIND_VERSION=v4.3.1
# renovate: datasource=npm depName=esbuild
ARG ESBUILD_VERSION=0.24.0
ARG TARGETARCH
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
# The Tailwind standalone binary is glibc (hence the debian base, not alpine).
# esbuild ships its platform binary as an npm tarball we extract with plain curl
# + tar (no npm client). Map the docker arch to each project's asset name.
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) tw_arch=x64;  es_arch=x64  ;; \
      arm64) tw_arch=arm64; es_arch=arm64 ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${tw_arch}"; \
    chmod +x /usr/local/bin/tailwindcss; \
    curl -fsSL "https://registry.npmjs.org/@esbuild/linux-${es_arch}/-/linux-${es_arch}-${ESBUILD_VERSION}.tgz" \
      | tar -xzO package/bin/esbuild > /usr/local/bin/esbuild; \
    chmod +x /usr/local/bin/esbuild
WORKDIR /lio
# app.css @source globs scan view/**/*.templ and ../cmd/lio/static/*.js, so both
# trees must be present when the CLI tree-shakes to the used utility classes.
COPY view ./view
COPY cmd/lio/static ./cmd/lio/static
# Build app.css, concatenate the board+piece theme stylesheets into a single
# themes.css (8 board + 3 piece source files → 1 request; see themeStyles in
# view/scripts.templ), then minify our hand-authored classic scripts in place
# (esbuild without --bundle preserves global/top-level names, so the window.*
# interface between files is unchanged). htmx.min.js / octadground.js are already
# minified.
RUN mkdir -p /out \
    && tailwindcss -i view/app.css -o /out/app.css --minify \
    && cat cmd/lio/static/res/themes/board/*.css cmd/lio/static/res/themes/piece/*.css > /out/themes.css \
    && for f in lio lio-game lio-tv lio-room-create lio-home-demo lio-about lio-auth; do \
         esbuild "cmd/lio/static/$f.js" --minify --outfile="/out/$f.js"; \
       done

# ---- build: compile the Go binary, embedding the freshly built assets
FROM golang:1.26.4-alpine3.24 AS build

# short git commit hash of the source being built, injected into config.Revision
# so the running binary reports its exact build in the footer. The build context
# is src/ (no .git), so the deploy tooling passes this in — see deploy/deploy-fly.sh.
ARG GIT_REV=""

# ca-certificates is installed here purely so the scratch runtime stage can copy
# the CA bundle for the app's outbound TLS (object store). Modules are vendored,
# so the build itself needs no network.
RUN apk add --no-cache ca-certificates

WORKDIR /build
COPY . .
# Overlay the generated assets (gitignored, so absent from the build context).
# octadground.min.js is served as octadground.js (see view/scripts.templ).
COPY --from=assets-octadground /og/dist/octadground.min.js cmd/lio/static/octadground.js
COPY --from=assets-octadground /og/assets/octadground.base.css cmd/lio/static/octadground.base.css
# /out holds app.css + the minified lio*.js; overlay them onto the static dir.
COPY --from=assets-web /out/ cmd/lio/static/

WORKDIR cmd/lio
RUN CGO_ENABLED=0 go build -v -trimpath \
    -ldflags "-X github.com/dechristopher/lio/config.Revision=${GIT_REV}" \
    -o lio

# ---- final packaging: scratch (no OS), non-root, self-checking
# The binary is CGO_ENABLED=0 (fully static), so it runs on an empty image with
# nothing but the CA bundle it needs for outbound TLS to the object store. No
# shell, no package manager, no libc, no busybox — the smallest possible runtime
# and attack surface. This is safe here because the app was verified to do no
# user lookups (so a numeric uid needs no /etc/passwd), touch no /tmp, and report
# its own health (so no wget is needed — see the HEALTHCHECK below). Debug flags
# are set per-deploy (compose `command:`), never baked in.
FROM scratch

# CA roots for outbound TLS (object store). Go's crypto/x509 reads this path.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=build /build/cmd/lio/lio /lio

# run unprivileged. A bare numeric uid:gid needs no /etc/passwd entry because the
# app never resolves its own user by name.
USER 10001:10001

EXPOSE 4444

# In-binary healthcheck: scratch has no shell/wget, so probe via `/lio --health`
# (GET /lio on the loopback-only internal health listener, 127.0.0.1:4445 by
# default — never exposed outside the container; see cmd/lio/main.go and
# www/health.go). Compose overrides this with its own healthcheck of the same
# command; this covers standalone `docker run`.
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/lio", "--health"]

ENTRYPOINT ["/lio"]
