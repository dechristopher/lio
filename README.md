# [lioctad.org](https://lioctad.org)
[![Go Report Card](https://goreportcard.com/badge/github.com/dechristopher/lio)](https://goreportcard.com/report/github.com/dechristopher/lio)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://raw.githubusercontent.com/dechristopher/lio/master/LICENSE)

Lioctad (li[bre] octad) is a free online octad game server focused on
[realtime](https://lioctad.org/games) gameplay and ease of use.

Lioctad is written in Go 1.18 using Go Fiber and React 17 with Redux. Go
templates are used for templating. Pure octad logic is contained in the
[octad](https://github.com/dechristopher/octad) library. The server is fully
asynchronous, making heavy use of Go routines. WebSocket connections are handled
by a separate server that communicates using Redis PubSub. Lioctad talks to
an octad engine that uses Minimax with alpha-beta pruning for games against
computers. It uses PostgreSQL to store games, which are indexed by Elasticsearch.
HTTP requests and WebSocket connections can be proxied by Nginx. The frontend is
written in TypeScript, using Sass to generate CSS. All rated games are published
in a free PGN [database](https://lioctad.org/db).

## License

Lioctad is licensed under the GNU Affero General Public License 3 or any later
version at your choice. See COPYING for details.
