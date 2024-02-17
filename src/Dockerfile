# build stage
FROM golang:1.22.0-alpine3.19 AS build

WORKDIR /build
COPY . .

WORKDIR cmd/lio
RUN CGO_ENABLED=0 go build -v -trimpath -o lio

# final packaging
FROM golang:1.22.0-alpine3.19

# Run this here to ensure we always get up to date root certs
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /
COPY --from=build /build/cmd/lio/lio lio

EXPOSE 4444
ENTRYPOINT ["/lio", "--debug", "room,dispatch,clock"]
