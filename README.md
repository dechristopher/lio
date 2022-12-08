# [lioctad.org](https://lioctad.org)

[![Go Report Card](https://goreportcard.com/badge/github.com/dechristopher/lio)](https://goreportcard.com/report/github.com/dechristopher/lio)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://raw.githubusercontent.com/dechristopher/lio/master/LICENSE)

Lioctad (li[bre] octad) is a free online octad game server focused on
[realtime](https://lioctad.org/games) gameplay and ease of use.

Lioctad is written in Go 1.19 using Go Fiber and React 18 with NextJs. Pure octad logic is contained in the
[octad](https://github.com/dechristopher/octad) library. The server is fully
asynchronous, making heavy use of Go routines. WebSocket connections are handled
by a separate server that communicates using Redis PubSub. Lioctad talks to
an octad engine that uses Minimax with alpha-beta pruning for games against
computers. It uses PostgreSQL to store games, which are indexed by Elasticsearch.
HTTP requests and WebSocket connections can be proxied by Nginx. The frontend is
written in TypeScript, using Sass to generate CSS. All rated games are published
in a free PGN [database](https://lioctad.org/db).

## Developer Setup

### Buf CLI

The Buf CLI enables us to create consistent Protobuf APIs. It handles compilation of `.proto` files, linting, and the detection of breaking changes.

You can find installation instructions [here](https://docs.buf.build/installation) and a guided tour of Buf [here](https://docs.buf.build/tour/introduction).

### Commands

The following commands should be run from the root of the repository.

The buf generation command is setup as an NPM script because the TypeScript compiler plugin depends on a couple runtime packages; See [how to generate](https://github.com/bufbuild/protobuf-es/blob/main/docs/generated_code.md#how-to-generate).

Do **NOT** forget to run `npm install`!

| Command           | Result                                                                                              |
| ----------------- | --------------------------------------------------------------------------------------------------- |
| `npm run buf:gen` | Compiles `.proto` files into new API code for Lioctad services.                                     |
| `buf lint`        | Lints all `.proto` files against the [DEFAULT](https://docs.buf.build/lint/rules#default) rule set. |

## License

Lioctad is licensed under the GNU Affero General Public License 3 or any later
version at your choice. See COPYING for details.
