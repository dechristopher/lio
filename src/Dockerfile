# build stage
FROM golang:1.15.7-alpine3.13 AS build

WORKDIR /build
COPY . .

RUN CGO_ENABLED=0 go build -v -trimpath -o lioctad

# final packaging
FROM scratch

WORKDIR /
COPY --from=build /build/lioctad lioctad
COPY --from=build /build/static static/
COPY --from=build /build/views views/

EXPOSE 4444
ENTRYPOINT ["/lioctad"]