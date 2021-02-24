# build stage
FROM golang:1.16.0-alpine3.13 AS build

WORKDIR /build
COPY . .

RUN CGO_ENABLED=0 go build -v -trimpath -o lioctad

# final packaging
FROM scratch

WORKDIR /
COPY --from=build /build/lioctad lioctad
# Temporary until gofiber merges:
# https://github.com/gofiber/fiber/pull/1189
COPY --from=build /build/static/res/ico/favicon.ico favicon.ico

EXPOSE 4444
ENTRYPOINT ["/lioctad"]