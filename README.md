# [lioctad](https://lioctad.org)

Lioctad (li[bre] octad) is a free online chess game server focused on [realtime](https://lioctad.org/games) gameplay and ease of use.

Lioctad is written in Go 1.16 using Go Fiber and React 17 with Redux. Go templates are used for templating. Pure octad logic is contained in the [octad](https://github.com/dechristopher/octad) library. The server is fully asynchronous, making heavy use of Go routines. WebSocket connections are handled by a separate server that communicates using Redis PubSub. Lioctad will talk to an octad engine that has yet to be developed for games against computers. It uses PostgreSQL to store games, which are indexed by Elasticsearch. HTTP requests and WebSocket connections can be proxied by Nginx. The frontend is written in TypeScript, using Sass to generate CSS. All rated games are published in a free PGN [database](https://lioctad.org/db).

## License

Lioctad is licensed under the GNU Affero General Public License 3 or any later version at your choice. See COPYING for details.
