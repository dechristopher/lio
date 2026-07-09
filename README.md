# [lioctad.org](https://lioctad.org)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://raw.githubusercontent.com/dechristopher/lio/master/LICENSE)

Lioctad (li[bre] octad) is a free online octad game server focused on
[realtime](https://lioctad.org/games) gameplay and ease of use.

**Octad** is a 4x4 chess variant: the same pieces and rules as chess (check,
checkmate, promotion, en passant) with variant-specific castling, played on a
4x4 board. The default starting position in OFEN (Octad's FEN) is
`ppkn/4/4/NKPP w NCFncf - 0 1`.

## Stack

Lioctad is written in Go 1.25 using [Fiber](https://gofiber.io/). Templ is used for templating,
with HTMX for reactivity. Pure octad logic (move generation, legality, OFEN
parsing, and outcomes) lives in the [octad](https://github.com/dechristopher/octad) library, not in this repo; the
server is built around it. The server is fully asynchronous, making heavy use of
Go routines. WebSocket connections are handled by a separate server that
communicates using Redis PubSub. Lioctad.org talks to an octad engine that uses
minimax with alpha-beta pruning for games against computers. It uses PostgreSQL
to store games, which are indexed by Elasticsearch. HTTP requests and WebSocket
connections can be proxied by Nginx. All rated games are published in a free PGN
[database](https://lioctad.org/db).

## Repository layout

This is a monorepo:

- **`src/`** — the Go backend (module `github.com/dechristopher/lio`).
  Dependencies are vendored. Notable packages: `cmd/lio` (entrypoint), `engine`
  (Octad search + evaluation), `dispatch` (engine move requests), `room` (game
  state machine), `www` (HTTP/WebSocket server).

## Development

Backend (run from `src/`):

```bash
go build ./...                                      # build
go test ./engine/ -count=1                          # run engine tests
go run ./cmd/lio --debug room,dispatch,clock,engine # run the server locally
```

The server binary accepts `--debug <comma,separated,flags>` to enable scoped
debug logging. By default, it listens on port `4444`.

## License

Lioctad is licensed under the GNU Affero General Public License 3 or any later
version of your choice. See COPYING for details.
