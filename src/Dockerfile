# build stage
FROM golang:1.18.3-alpine3.16 AS build

WORKDIR /build
COPY . .

RUN CGO_ENABLED=0 go build -v -trimpath -o lioctad

# final packaging
FROM golang:1.18.3-alpine3.16

# Run this here to ensure we always get up to date root certs
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /
COPY --from=build /build/lioctad lioctad

EXPOSE 4444
ENTRYPOINT ["/lioctad"]
