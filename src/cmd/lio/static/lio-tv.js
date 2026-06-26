// lio-tv.js — home-page "live games" TV widget.
//
// A self-contained, read-only WebSocket client for the global /socket/tv
// channel. It receives a one-shot snapshot of the featured games on connect,
// then a stream of add / move / remove deltas, and renders each game as a small
// octadground board (viewOnly) with thin clock progress bars and the match
// score. It owns its own connection (jittered reconnect + stale-socket
// watchdog), so it needs neither lio.js nor howler.
(function () {
	const grid = document.getElementById('tv-grid');
	const emptyEl = document.getElementById('tv-empty');
	const statusEl = document.getElementById('tv-status');
	if (!grid || typeof Octadground === 'undefined') {
		return;
	}

	// roomID -> slot { card, og, white, black, variantEl, scoreEl,
	//                  control, wt, bt, toMove, at, over }
	const slots = new Map();

	// ---- connection: jittered backoff + stale-socket watchdog (cf. lio.js) ----
	let ws = null;
	let stopped = false;
	let attempts = 0;
	let pingTimer = null;
	let pingsSincePong = 0;
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
		ws = new WebSocket(location.origin.replace(/^http/, 'ws') + '/socket/tv');
		ws.onopen = () => {
			attempts = 0;
			pingsSincePong = 0;
			schedulePing(500);
		};
		ws.onclose = () => {
			ws = null;
			clearTimeout(pingTimer);
			pingsSincePong = 0;
			if (stopped) {
				return;
			}
			setStatus('reconnecting…');
			reconnect();
		};
		ws.onmessage = (evt) => handle(evt.data);
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
		// pong (latency frame) resets the watchdog
		if (msg.po && msg.po === 1) {
			pingsSincePong = 0;
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
		if (d.d) {        // remove: a room's slot was freed
			removeSlot(d.d);
		}
		refreshEmpty();
	};

	const rebuild = (list) => {
		slots.forEach((slot) => destroyBoard(slot));
		slots.clear();
		grid.innerHTML = '';
		(list || []).forEach(upsert);
	};

	const upsert = (g) => {
		let slot = slots.get(g.r);
		if (!slot) {
			slot = createSlot(g);
			slots.set(g.r, slot);
		}
		updateSlot(slot, g);
	};

	const removeSlot = (room) => {
		const slot = slots.get(room);
		if (!slot) {
			return;
		}
		destroyBoard(slot);
		slot.card.remove();
		slots.delete(room);
	};

	const destroyBoard = (slot) => {
		try {
			slot.og.destroy();
		} catch (e) { /* ignore */ }
	};

	// ---- DOM + board construction ----
	const clockEl = () => {
		const root = document.createElement('div');
		root.className = 'tv-clock';
		const bar = document.createElement('div');
		bar.className = 'tv-bar';
		const fill = document.createElement('i');
		bar.appendChild(fill);
		const time = document.createElement('span');
		time.className = 'tv-time';
		time.textContent = '0:00';
		root.appendChild(bar);
		root.appendChild(time);
		return {root, fill, time};
	};

	const createSlot = (g) => {
		const card = document.createElement('a');
		card.className = 'tv-card';
		card.href = '/' + g.r;
		card.dataset.room = g.r;

		// boards orient white-at-bottom, so the top clock is Black, bottom White
		const black = clockEl();
		const white = clockEl();

		const board = document.createElement('div');
		board.className = 'tv-board gcon';
		const gwrap = document.createElement('div');
		gwrap.className = 'gwrap green alpha';
		const ogWrap = document.createElement('div');
		ogWrap.className = 'og-wrap';
		gwrap.appendChild(ogWrap);
		board.appendChild(gwrap);

		const info = document.createElement('div');
		info.className = 'tv-info';
		const variantEl = document.createElement('span');
		variantEl.className = 'tv-variant';
		const scoreEl = document.createElement('span');
		scoreEl.className = 'tv-score';
		info.appendChild(variantEl);
		info.appendChild(scoreEl);

		card.appendChild(black.root);
		card.appendChild(board);
		card.appendChild(white.root);
		card.appendChild(info);
		grid.appendChild(card);

		const og = Octadground(ogWrap, {
			ofen: boardOf(g.o),
			orientation: 'white',
			viewOnly: true,
			coordinates: false,
			highlight: {lastMove: true},
			drawable: {enabled: false},
			lastMove: lastMoveOf(g.l)
		});

		return {
			card, og, white, black, variantEl, scoreEl,
			control: g.tc, wt: g.w, bt: g.b, toMove: 'w', at: Date.now(),
			over: false, running: false
		};
	};

	const updateSlot = (slot, g) => {
		slot.control = g.tc;
		slot.wt = g.w;
		slot.bt = g.b;
		slot.toMove = sideToMove(g.o);
		slot.at = Date.now();
		slot.over = !!g.x;
		// the clock only ticks once the first move has started it; until then
		// hold the clocks static at their full value
		slot.running = !!g.rn;

		slot.og.set({
			ofen: boardOf(g.o),
			lastMove: lastMoveOf(g.l),
			turnColor: slot.toMove === 'w' ? 'white' : 'black'
		});

		slot.variantEl.textContent = g.vn || 'Octad';
		slot.card.title = (g.vb ? 'vs Computer · ' : '') + (g.vn || 'Octad');

		const sw = g.sc ? (g.sc.w || 0) : 0;
		const sb = g.sc ? (g.sc.b || 0) : 0;
		slot.scoreEl.textContent = sw + ' – ' + sb;

		slot.card.classList.toggle('over', slot.over);

		paintClock(slot.white, slot.control, slot.wt, !slot.over && slot.toMove === 'w');
		paintClock(slot.black, slot.control, slot.bt, !slot.over && slot.toMove === 'b');
	};

	const paintClock = (c, control, centis, running) => {
		centis = Math.max(centis, 0);
		c.fill.style.width = barPct(control, centis);
		c.time.textContent = fmtTime(centis);
		c.root.classList.toggle('run', running);
		c.root.classList.toggle('low', centis < 1000); // < 10s
	};

	// one shared ticker decrements the active side on every board (one timer, not
	// one per board); the next move delta resets `at` + clocks from the server
	setInterval(() => {
		const now = Date.now();
		slots.forEach((slot) => {
			// don't tick a finished game, or one whose clock hasn't started yet
			// (pre-first-move): both should hold their displayed time static
			if (slot.over || !slot.running) {
				return;
			}
			const running = slot.toMove === 'w' ? slot.white : slot.black;
			const base = slot.toMove === 'w' ? slot.wt : slot.bt;
			const remaining = Math.max(base - (now - slot.at) / 10, 0);
			running.fill.style.width = barPct(slot.control, remaining);
			running.time.textContent = fmtTime(remaining);
			running.root.classList.toggle('low', remaining < 1000);
		});
	}, 250);

	const refreshEmpty = () => {
		const n = slots.size;
		if (emptyEl) {
			emptyEl.classList.toggle('hidden', n > 0);
		}
		grid.classList.toggle('hidden', n === 0);
		setStatus(n > 0 ? (n + ' live') : 'no games');
	};

	// ---- helpers ----
	const boardOf = (ofen) => (ofen || '').split(' ')[0];
	const sideToMove = (ofen) => ((ofen || '').split(' ')[1] === 'b' ? 'b' : 'w');
	const lastMoveOf = (uoi) =>
		(uoi && uoi.length >= 4) ? [uoi.substring(0, 2), uoi.substring(2, 4)] : [];
	const barPct = (control, centis) =>
		(control > 0 ? Math.min((centis / control) * 100, 100) : 0) + '%';
	const fmtTime = (centis) => {
		const total = Math.floor(centis / 100);
		const m = Math.floor(total / 60);
		const s = total % 60;
		return m + ':' + (s < 10 ? '0' + s : s);
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
