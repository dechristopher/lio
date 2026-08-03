// lio-tv.js — home-page "live games" TV widget.
//
// A self-contained, read-only WebSocket client for the global /socket/home
// channel. It receives a one-shot snapshot of the featured games on connect,
// then a stream of add / move / remove deltas, and renders each one as a
// lio-miniboard card. It owns its own connection (jittered reconnect +
// stale-socket watchdog), so it needs neither lio.js nor howler.
//
// Everything the cards themselves do — the boards, clocks, deploy covers,
// countdown dials and result overlays — lives in lio-miniboard.js, which this
// file and the username hover card (lio-card.js) both drive. What is left here
// is the grid: the socket, the featured-slot bookkeeping, and the page chrome
// around it.
(function () {
	const grid = document.getElementById('tv-grid');
	const emptyEl = document.getElementById('tv-empty');
	const statusEl = document.getElementById('tv-status');
	const liveDot = document.getElementById('tv-live-dot');
	if (!grid || !window.lioMiniboard || typeof Octadground === 'undefined') {
		return;
	}
	const mini = window.lioMiniboard;

	// roomID -> the miniboard handle holding that room's slot
	const slots = new Map();

	// ---- connection: jittered backoff + stale-socket watchdog (cf. lio.js) ----
	let ws = null;
	let stopped = false;
	let attempts = 0;
	let pingTimer = null;
	let pingsSincePong = 0;
	// latency tracking for the shared header connection indicator (window.lioConn)
	let lastPingTime = 0;
	let latency = 0;
	let pongCount = 0;
	const pingDelay = 5000;
	const maxMissedPongs = 3;
	const reconnectBaseMs = 1000;
	const reconnectCapMs = 30000;

	const setStatus = (t) => {
		if (statusEl) {
			statusEl.textContent = t;
		}
	};

	const connect = () => {
		// Claim the page's one socket, so lio-notify.js does not open a second
		// one on the home page. The TV stream is a broadcast channel, but the
		// server addresses a notification to a single connection on it, so a
		// viewer only ever receives their own (arch/NOTIFICATIONS.md).
		window.lioSocketOwner = 'tv';
		ws = new WebSocket(location.origin.replace(/^http/, 'ws') + '/socket/home');
		ws.onopen = () => {
			attempts = 0;
			pingsSincePong = 0;
			if (window.lioConn) {
				window.lioConn.set('online');
			}
			// hand the hover card this socket to watch rooms over
			// (arch/PLAYER_CARD.md); it holds no connection of its own
			if (window.lioCard) {
				window.lioCard.wire(sendJSON);
			}
			schedulePing(500);
		};
		ws.onclose = () => {
			ws = null;
			clearTimeout(pingTimer);
			pingsSincePong = 0;
			if (window.lioCard) {
				window.lioCard.wire(null);
			}
			if (stopped) {
				return;
			}
			setStatus('reconnecting…');
			if (window.lioConn) {
				window.lioConn.set('reconnecting');
			}
			reconnect();
		};
		ws.onmessage = (evt) => handle(evt.data);
	};

	// sendJSON is handed to the hover card as its outbound path. It reports
	// whether the frame actually went out, so a caller can fall back to the
	// static snapshot it already has rather than waiting for a stream that will
	// never start.
	const sendJSON = (obj) => {
		try {
			if (ws && ws.readyState === WebSocket.OPEN) {
				ws.send(JSON.stringify(obj));
				return true;
			}
		} catch (e) { /* ignore */ }
		return false;
	};

	const reconnect = () => {
		attempts++;
		const ceil = Math.min(reconnectCapMs, reconnectBaseMs * Math.pow(2, attempts));
		setTimeout(connect, Math.random() * ceil);
	};

	const schedulePing = (delay) => {
		clearTimeout(pingTimer);
		pingTimer = setTimeout(ping, delay);
	};

	const ping = () => {
		// a half-open socket fires no onclose on its own; force a reconnect once
		// enough pings have gone unanswered
		if (pingsSincePong >= maxMissedPongs) {
			if (ws) {
				ws.close(4000, 'stale connection');
			}
			return;
		}
		try {
			if (ws && ws.readyState === WebSocket.OPEN) {
				ws.send(JSON.stringify({pi: 1}));
				lastPingTime = Date.now();
				pingsSincePong++;
			}
		} catch (e) { /* ignore */ }
		schedulePing(pingDelay);
	};

	// ---- message handling ----
	const handle = (raw) => {
		if (!raw) {
			return;
		}
		let msg;
		try {
			msg = JSON.parse(raw);
		} catch (e) {
			return;
		}
		// pong (latency frame) resets the watchdog and feeds the header indicator
		if (msg.po && msg.po === 1) {
			pingsSincePong = 0;
			const currentLag = Math.min(Date.now() - lastPingTime, 10000);
			pongCount++;
			// average the first few samples, then a weighted moving average (cf. lio.js)
			const weight = pongCount > 4 ? 0.1 : 1 / pongCount;
			latency += weight * (currentLag - latency);
			if (window.lioConn) {
				window.lioConn.set('online', latency);
			}
			return;
		}
		// server version hello: the home page loads no lio.js socket, so this
		// TV stream is where a deploy's new version first shows up — hand it
		// to the shared refresh prompt (updateNoticeScript in the header)
		// notification frame: this socket carries the viewer's own badge and
		// messages as well as the grid (arch/NOTIFICATIONS.md)
		if (msg.t === 'nt') {
			if (msg.d && window.lioNotify) {
				window.lioNotify.apply(msg.d);
			}
			return;
		}
		// the reconnect bar: this socket is the home page's only one, so it is
		// where a viewer browsing the lobby learns their game just started or
		// ended (arch/ONE_GAME_AT_A_TIME.md)
		if (msg.t === 'lg') {
			if (window.lioLiveGame) {
				window.lioLiveGame.apply(msg.d || {});
			}
			return;
		}
		if (msg.t === 'si') {
			if (msg.d && msg.d.v && window.lioUpdateNotice) {
				window.lioUpdateNotice(msg.d.v);
			}
			return;
		}
		// activity digest: the stat tiles, open challenges and players panel
		// below the grid. They share this socket rather than opening a second
		// one — the page holds exactly one connection, which is what the
		// site-wide presence walk counts (arch/HOME_ACTIVITY_STREAMING.md).
		if (msg.t === 'hm') {
			if (msg.d && window.lioHomeActivity) {
				window.lioHomeActivity.apply(msg.d);
			}
			return;
		}
		// the header following badge's count. Separate from the digest's own
		// Following section: that one draws the players card on this page, this
		// one drives a control that is on every page.
		if (msg.t === 'fo') {
			if (msg.d) {
				window.__lioFollowOnline = msg.d.o;
				if (window.lioFollowBadge) {
					window.lioFollowBadge.apply(msg.d.o);
				}
			}
			return;
		}
		// a watched room's state, for the hover card. It rides this socket
		// because the page holds exactly one, and it is addressed to this
		// connection alone rather than broadcast — the grid ignores it, which is
		// why it is a tag of its own rather than another TVPayload field
		// (arch/PLAYER_CARD.md).
		if (msg.t === 'wg') {
			if (window.lioCard) {
				window.lioCard.live(msg.d || {});
			}
			return;
		}
		if (msg.t !== 'tv' || !msg.d) {
			return;
		}
		const d = msg.d;
		if (d.s) {        // snapshot: the full featured set
			rebuild(d.s);
		}
		if (d.a) {        // add: a game entered a slot (or a rematch replaced one)
			upsert(d.a);
		}
		if (d.m) {        // move: a featured game advanced or ended
			upsert(d.m);
		}
		if (d.w) {        // crowd: a featured room's spectator count changed
			const slot = slots.get(d.w.r);
			if (slot) {
				mini.watchers(slot, d.w.n);
			}
		}
		if (d.d) {        // remove: a room's slot was freed
			removeSlot(d.d);
		}
		refreshEmpty();
	};

	const rebuild = (list) => {
		slots.forEach((slot) => mini.destroy(slot));
		slots.clear();
		grid.innerHTML = '';
		(list || []).forEach(upsert);
	};

	const upsert = (g) => {
		let slot = slots.get(g.r);
		if (!slot) {
			slot = mini.create(g, {href: '/' + g.r});
			grid.appendChild(slot.el);
			slots.set(g.r, slot);
		}
		mini.update(slot, g);
	};

	const removeSlot = (room) => {
		const slot = slots.get(room);
		if (!slot) {
			return;
		}
		mini.destroy(slot);
		slot.el.remove();
		slots.delete(room);
	};

	const refreshEmpty = () => {
		const n = slots.size;
		if (emptyEl) {
			emptyEl.classList.toggle('hidden', n > 0);
		}
		if (liveDot) {
			liveDot.classList.toggle('hidden', n === 0);
		}
		grid.classList.toggle('hidden', n === 0);
		setStatus(n > 0 ? (n + ' live') : 'no games');
	};

	// stop reconnecting once the page is going away
	window.addEventListener('pagehide', () => {
		stopped = true;
		if (ws) {
			ws.close();
		}
	});

	connect();
})();

// The hidden-tab poll gate that used to live here is gone with the poll it
// gated. #home-activity no longer issues htmx requests at all: it is streamed
// over this socket (arch/HOME_ACTIVITY_STREAMING.md), and a backgrounded tab
// costs the server nothing because the digest is derived once for the whole
// site rather than once per viewer.
