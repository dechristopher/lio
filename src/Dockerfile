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
ARG OCTADGROUND_REF=v7.12.0
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

# ---- css: build the minified Tailwind stylesheet with the standalone CLI (no npm)
FROM debian:bookworm-slim AS assets-css
# renovate: datasource=github-releases depName=tailwindlabs/tailwindcss
ARG TAILWIND_VERSION=v4.3.1
ARG TARGETARCH
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
# The standalone binary is glibc (hence the debian base, not alpine). Map the
# docker arch to Tailwind's release asset name.
RUN case "${TARGETARCH}" in \
      amd64) tw_arch=x64 ;; \
      arm64) tw_arch=arm64 ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${tw_arch}" \
    && chmod +x /usr/local/bin/tailwindcss
WORKDIR /lio
# app.css @source globs scan view/**/*.templ and ../cmd/lio/static/*.js, so both
# trees must be present when the CLI tree-shakes to the used utility classes.
COPY view ./view
COPY cmd/lio/static ./cmd/lio/static
RUN tailwindcss -i view/app.css -o /app.css --minify

# ---- build: compile the Go binary, embedding the freshly built assets
FROM golang:1.26.4-alpine3.24 AS build

# short git commit hash of the source being built, injected into config.Revision
# so the running binary reports its exact build in the footer. The build context
# is src/ (no .git), so the deploy tooling passes this in — see deploy/deploy-fly.sh.
ARG GIT_REV=""

WORKDIR /build
COPY . .
# Overlay the generated assets (gitignored, so absent from the build context).
# octadground.min.js is served as octadground.js (see view/scripts.templ).
COPY --from=assets-octadground /og/dist/octadground.min.js cmd/lio/static/octadground.js
COPY --from=assets-octadground /og/assets/octadground.base.css cmd/lio/static/octadground.base.css
COPY --from=assets-css /app.css cmd/lio/static/app.css

WORKDIR cmd/lio
RUN CGO_ENABLED=0 go build -v -trimpath \
    -ldflags "-X github.com/dechristopher/lio/config.Revision=${GIT_REV}" \
    -o lio

# ---- final packaging
FROM golang:1.26.4-alpine3.24

# Run this here to ensure we always get up to date root certs
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /
COPY --from=build /build/cmd/lio/lio lio

EXPOSE 4444
ENTRYPOINT ["/lio", "--debug", "room,dispatch,clock"]
