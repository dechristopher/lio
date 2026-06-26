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

	// roomID -> slot { card, og, top, bottom, whiteEl, blackEl, variantEl,
	//                  control, wt, bt, toMove, at, over, running, orient,
	//                  gameId, sw, sb }
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
		ws = new WebSocket(location.origin.replace(/^http/, 'ws') + '/socket/tv');
		ws.onopen = () => {
			attempts = 0;
			pingsSincePong = 0;
			if (window.lioConn) {
				window.lioConn.set('online');
			}
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
			if (window.lioConn) {
				window.lioConn.set('reconnecting');
			}
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
	// small cpu glyph (mirrors view.iconCpu) marking a bot-played side; shown only
	// when the clock's .tv-clock root carries .is-bot
	const CPU_ICON =
		'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
		'<rect x="4" y="4" width="16" height="16" rx="2" ry="2"></rect>' +
		'<rect x="9" y="9" width="6" height="6"></rect>' +
		'<path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3"></path></svg>';

	// a clock row is [bot icon][time bar][time][side score]; the score sits at the
	// row end so it reads as a per-side number, and the bot icon leads the row
	const clockEl = () => {
		const root = document.createElement('div');
		root.className = 'tv-clock';
		const bot = document.createElement('span');
		bot.className = 'tv-bot';
		bot.title = 'Computer player';
		bot.innerHTML = CPU_ICON;
		const bar = document.createElement('div');
		bar.className = 'tv-bar';
		const fill = document.createElement('i');
		bar.appendChild(fill);
		const time = document.createElement('span');
		time.className = 'tv-time';
		time.textContent = '0:00';
		const score = document.createElement('span');
		score.className = 'tv-side-score';
		score.textContent = '0';
		root.appendChild(bot);
		root.appendChild(bar);
		root.appendChild(time);
		root.appendChild(score);
		return {root, bot, fill, time, score};
	};

	const createSlot = (g) => {
		const card = document.createElement('a');
		card.className = 'tv-card';
		card.href = '/' + g.r;
		card.dataset.room = g.r;

		// the two clock rows are fixed in place (top above the board, bottom below);
		// updateSlot maps each color to a row from g.or so the anchored player keeps
		// the bottom row while the board flips between games
		const top = clockEl();
		const bottom = clockEl();

		const board = document.createElement('div');
		board.className = 'tv-board gcon';
		const gwrap = document.createElement('div');
		gwrap.className = 'gwrap green alpha';
		const ogWrap = document.createElement('div');
		ogWrap.className = 'og-wrap';
		gwrap.appendChild(ogWrap);
		board.appendChild(gwrap);

		// per-side score now lives on each clock row, so the caption is variant-only
		const info = document.createElement('div');
		info.className = 'tv-info';
		const variantEl = document.createElement('span');
		variantEl.className = 'tv-variant';
		info.appendChild(variantEl);

		card.appendChild(top.root);
		card.appendChild(board);
		card.appendChild(bottom.root);
		card.appendChild(info);
		grid.appendChild(card);

		const orient = orientOf(g);
		const og = Octadground(ogWrap, {
			ofen: boardOf(g.o),
			orientation: orient === 'w' ? 'white' : 'black',
			viewOnly: true,
			coordinates: false,
			highlight: {lastMove: true},
			drawable: {enabled: false},
			lastMove: lastMoveOf(g.l)
		});

		return {
			card, og, top, bottom, variantEl,
			// whiteEl/blackEl: which fixed row currently holds each color; remapped
			// by updateSlot as the anchored side flips between games
			whiteEl: orient === 'w' ? bottom : top,
			blackEl: orient === 'w' ? top : bottom,
			control: g.tc, wt: g.w, bt: g.b, toMove: 'w', at: Date.now(),
			over: false, running: false, orient: orient,
			// gameId + last-seen scores drive the end-of-game score flash and its
			// reset when a rematch backfills the same slot
			gameId: g.i, sw: scoreOf(g, 'w'), sb: scoreOf(g, 'b')
		};
	};

	const updateSlot = (slot, g) => {
		// anchored color sits in the bottom row; remap each color to its fixed row
		// so the anchored player keeps the bottom seat while the board itself flips
		slot.orient = orientOf(g);
		slot.whiteEl = slot.orient === 'w' ? slot.bottom : slot.top;
		slot.blackEl = slot.orient === 'w' ? slot.top : slot.bottom;

		// a new game id in this slot (rematch/backfill) → clear any stale flash and
		// re-baseline the scores so only the next end-of-game delta animates
		if (g.i && g.i !== slot.gameId) {
			slot.gameId = g.i;
			clearScoreFlash(slot.top);
			clearScoreFlash(slot.bottom);
			slot.sw = scoreOf(g, 'w');
			slot.sb = scoreOf(g, 'b');
		}

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
			turnColor: slot.toMove === 'w' ? 'white' : 'black',
			// re-orient on rematch so the board flips to keep the anchored side down
			orientation: slot.orient === 'w' ? 'white' : 'black'
		});

		slot.variantEl.textContent = g.vn || 'Octad';
		slot.card.title = (g.vb ? 'vs Computer · ' : '') + (g.vn || 'Octad');

		// mark the bot's side by color → whichever row currently holds it; both
		// rows are set every update so a flip never leaves a stale icon
		slot.whiteEl.root.classList.toggle('is-bot', g.bc === 'w');
		slot.blackEl.root.classList.toggle('is-bot', g.bc === 'b');

		// per-side score, flashing the delta at game end: green +1 (a win), grey
		// +½ (a draw). Score only changes at game end, so a positive delta is the
		// natural trigger.
		const sw = scoreOf(g, 'w');
		const sb = scoreOf(g, 'b');
		applyScore(slot.whiteEl, sw, sw - slot.sw);
		applyScore(slot.blackEl, sb, sb - slot.sb);
		slot.sw = sw;
		slot.sb = sb;

		slot.card.classList.toggle('over', slot.over);

		paintClock(slot.whiteEl, slot.control, slot.wt, !slot.over && slot.toMove === 'w');
		paintClock(slot.blackEl, slot.control, slot.bt, !slot.over && slot.toMove === 'b');
	};

	// applyScore writes a side's score and, on an increase, pulses it (green for a
	// win's +1, grey for a draw's +½)
	const applyScore = (c, value, delta) => {
		c.score.textContent = value;
		if (delta > 0) {
			flashScore(c, delta);
		}
	};

	const flashScore = (c, delta) => {
		c.score.classList.remove('score-win', 'score-draw');
		void c.score.offsetWidth; // reflow so re-adding the class restarts the animation
		c.score.classList.add(delta >= 0.75 ? 'score-win' : 'score-draw');
	};

	const clearScoreFlash = (c) => c.score.classList.remove('score-win', 'score-draw');

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
			const running = slot.toMove === 'w' ? slot.whiteEl : slot.blackEl;
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
	const scoreOf = (g, side) => (g.sc ? (g.sc[side] || 0) : 0);
	// color anchored to the board bottom (server-chosen, stable across flips);
	// default white-at-bottom when absent (e.g. older payloads)
	const orientOf = (g) => (g.or === 'b' ? 'b' : 'w');
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
