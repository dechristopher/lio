# build stage
FROM golang:1.18.3-alpine3.16 AS build

WORKDIR /build
COPY . .

RUN CGO_ENABLED=0 go build -v -trimpath -o lioctad

# final packaging
FROM scratch

WORKDIR /
COPY --from=build /build/lioctad lioctad

EXPOSE 4444
ENTRYPOINT ["/lioctad"]
