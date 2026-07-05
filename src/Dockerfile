# build stage
FROM golang:1.26.4-alpine3.24 AS build

# short git commit hash of the source being built, injected into config.Revision
# so the running binary reports its exact build in the footer. The build context
# is src/ (no .git), so the deploy tooling passes this in — see deploy/deploy-fly.sh.
ARG GIT_REV=""

WORKDIR /build
COPY . .

WORKDIR cmd/lio
RUN CGO_ENABLED=0 go build -v -trimpath \
    -ldflags "-X github.com/dechristopher/lio/config.Revision=${GIT_REV}" \
    -o lio

# final packaging
FROM golang:1.26.4-alpine3.24

# Run this here to ensure we always get up to date root certs
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /
COPY --from=build /build/cmd/lio/lio lio

EXPOSE 4444
ENTRYPOINT ["/lio", "--debug", "room,dispatch,clock"]
