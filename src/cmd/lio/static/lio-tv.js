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
	const liveDot = document.getElementById('tv-live-dot');
	if (!grid || typeof Octadground === 'undefined') {
		return;
	}

	// roomID -> slot { card, og, top, bottom, whiteEl, blackEl, variantEl,
	//                  raceEl, watch, watchCount, control, wt, bt, toMove, at,
	//                  over, running, orient, gameId, ended, resultTimer }
	// (top/bottom are the two seat strips built by clockEl)
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
		if (msg.t === 'si') {
			if (msg.d && msg.d.v && window.lioUpdateNotice) {
				window.lioUpdateNotice(msg.d.v);
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
				setWatchers(slot, d.w.n);
			}
		}
		if (d.d) {        // remove: a room's slot was freed
			removeSlot(d.d);
		}
		refreshEmpty();
	};

	const rebuild = (list) => {
		slots.forEach((slot) => destroySlot(slot));
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
		destroySlot(slot);
		slot.card.remove();
		slots.delete(room);
	};

	// tear a slot's live resources down: the board, and the result overlay's fade
	// timer, which would otherwise outlive the card it was going to fade
	const destroySlot = (slot) => {
		clearTimeout(slot.resultTimer);
		try {
			slot.og.destroy();
		} catch (e) { /* ignore */ }
	};

	// ---- DOM + board construction ----
	// eye glyph for the spectator count at the caption row's end; same stroke
	// style as the cpu glyph below
	const EYE_ICON =
		'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
		'<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path>' +
		'<circle cx="12" cy="12" r="3"></circle></svg>';

	// A side's strip is an identity row — [persona glyph][title badge][name]
	// [time|lock][score] — stacked over a full-width clock bar. The name takes
	// the slack between the badges and the time and ellipsizes there, so a
	// 20-char username never squeezes the clock. `below` puts the bar first so
	// the two strips' bars both sit against the board (see .tv-clock.below in
	// app.css). During the blind deploy the clock slot is given over to the
	// seat's lock state — the clocks are static and identical then, while
	// whether each side has committed is the only thing actually happening.
	const clockEl = (below) => {
		const root = document.createElement('div');
		root.className = below ? 'tv-clock below' : 'tv-clock';

		const seat = document.createElement('div');
		seat.className = 'tv-seat';
		// a bot seat leads with its difficulty persona's piece glyph, the same
		// avatar the room clock shows; hidden entirely for a human seat
		const glyph = document.createElement('span');
		glyph.className = 'tv-glyph hidden';
		// the site-wide accent title badge; .tv-title only tightens it for the strip
		const title = document.createElement('span');
		title.className = 'player-title tv-title hidden';
		const name = document.createElement('span');
		name.className = 'tv-name';
		const time = document.createElement('span');
		time.className = 'tv-time';
		time.textContent = '0:00';
		// deploy-phase stand-in for the clock: this seat's confirmation state
		const lock = document.createElement('span');
		lock.className = 'tv-lock hidden';
		const score = document.createElement('span');
		score.className = 'tv-side-score';
		score.textContent = '0';
		seat.appendChild(glyph);
		seat.appendChild(title);
		seat.appendChild(name);
		seat.appendChild(time);
		seat.appendChild(lock);
		seat.appendChild(score);

		const bar = document.createElement('div');
		bar.className = 'tv-bar';
		const fill = document.createElement('i');
		bar.appendChild(fill);

		root.appendChild(seat);
		root.appendChild(bar);
		return {root, glyph, title, name, fill, time, lock, score};
	};

	// questionBand builds one home-rank cover for the blind deploy: the same
	// four "?" cells (and the same .deploy-questions/.dq-cell classes) the room
	// board uses, so the two surfaces can never drift apart on what a hidden
	// rank looks like. The grid always covers *both* ranks — every TV viewer is
	// a spectator, and a spectator is shown neither arrangement.
	const questionBand = (bottom) => {
		const band = document.createElement('div');
		band.className = bottom ? 'deploy-questions deploy-questions-btm' : 'deploy-questions';
		band.setAttribute('aria-hidden', 'true');
		for (let i = 0; i < 4; i++) {
			const cell = document.createElement('span');
			cell.className = 'dq-cell';
			cell.textContent = '?';
			band.appendChild(cell);
		}
		return band;
	};

	// ---- finished-game vocabulary ----
	// Backend method codes -> the glyph and the words for a finished game. This
	// is the room's match-timeline vocabulary (tlGlyphs / resultReasons in
	// lio-game.js — keep the three in step); the home page loads none of that
	// file, so the grid carries its own copy. All text codepoints except time,
	// an inline SVG clock (U+231B renders as an emoji): its stroke is
	// currentColor, so it takes the result color like every other glyph.
	const CLOCK_ICON =
		'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" aria-hidden="true">' +
		'<circle cx="12" cy="12" r="9"></circle><path d="M12 7v5l3.5 2"></path></svg>';
	const RESULT_GLYPHS = {
		checkmate: '#',
		time: CLOCK_ICON,
		resignation: '⚑',
		stalemate: '=',
		insufficient: '=',
		agreement: '=',
		repetition: '=',
		moverule: '=',
		abandoned: '×',
	};
	const RESULT_METHODS = {
		checkmate: 'by checkmate',
		time: 'on time',
		resignation: 'by resignation',
		stalemate: 'by stalemate',
		insufficient: 'by insufficient material',
		agreement: 'by agreement',
		repetition: 'by repetition',
		moverule: 'by the 25-move rule',
		abandoned: 'by abandonment',
	};

	// how long the result overlay holds before it fades. Sized to sit inside the
	// server's between-games interlude (matchInterludeWindow, 5s — see
	// room/handle_04_game_over.go), so it has cleared before the next game's
	// covers and countdown come up. A room that is not advancing (a rematch
	// window, a bot game's analysis grace) simply keeps its frozen board after.
	const resultHoldMs = 3500;

	// the countdown ring, structurally identical to the room's pre-start dial
	// (see prestart-overlay in view/components.templ) so the two read as the same
	// object: a track circle under an accent progress circle whose round stroke
	// cap gives the sweep its rounded leading edge. Same 48-unit viewBox and
	// r=21, so DIAL_CIRC below is the same number lio-game.js uses.
	const DIAL_RING =
		'<svg class="tv-dial-ring" viewBox="0 0 48 48" aria-hidden="true">' +
		'<circle class="tv-dial-track" cx="24" cy="24" r="21"></circle>' +
		'<circle class="tv-dial-progress" cx="24" cy="24" r="21"></circle></svg>';

	// circumference of the ring (2π·21); must match the stroke-dasharray in
	// app.css, exactly as lio-game.js's preStartRingCirc does for the room
	const DIAL_CIRC = 131.95;

	// dialEl builds the mid-board countdown shared by both pre-game timers (the
	// deploy auto-fill, then the first-move grace).
	const dialEl = () => {
		const root = document.createElement('div');
		root.className = 'tv-dial';
		root.setAttribute('aria-hidden', 'true');
		const face = document.createElement('div');
		face.className = 'tv-dial-face';
		face.innerHTML = DIAL_RING;
		const num = document.createElement('div');
		num.className = 'tv-dial-num';
		face.appendChild(num);
		root.appendChild(face);
		return {root, progress: face.querySelector('.tv-dial-progress'), num};
	};

	// resultEl builds the between-games result overlay: the finished game's
	// win-method glyph on the pre-game dial's own disc, over a pill holding the
	// resulting match score. The disc is literally .tv-dial-face — the countdown
	// and the result take the board's center in turn, and reusing the face keeps
	// them the same object rather than two lookalikes that can drift.
	const resultEl = () => {
		const root = document.createElement('div');
		root.className = 'tv-result';
		root.setAttribute('aria-hidden', 'true');
		const face = document.createElement('div');
		face.className = 'tv-dial-face';
		const glyph = document.createElement('div');
		glyph.className = 'tv-result-glyph';
		face.appendChild(glyph);
		// white's points, an en dash, black's — each number underlined in the
		// piece color of the side it belongs to (see .tv-rs::after), which is
		// what lets the pair go unlabelled at this size
		const score = document.createElement('div');
		score.className = 'tv-result-score';
		const white = document.createElement('span');
		white.className = 'tv-rs tv-rs-w';
		const dash = document.createElement('span');
		dash.className = 'tv-rs-dash';
		dash.textContent = '–';
		const black = document.createElement('span');
		black.className = 'tv-rs tv-rs-b';
		score.appendChild(white);
		score.appendChild(dash);
		score.appendChild(black);
		root.appendChild(face);
		root.appendChild(score);
		return {root, glyph, white, black};
	};

	// setSeat writes a side's identity from the wire seat (proto.TVSeat): the
	// name, the account's title badge, and a bot's persona glyph. Everything is
	// written as text, never markup — a username and a title code are account
	// data. Both badges collapse when the seat carries neither, so an anonymous
	// human's row is just the name.
	const setSeat = (c, s, deploying) => {
		s = s || {};
		const label = s.n || 'Anonymous';
		const glyph = s.bot ? (s.g || '') : '';
		c.glyph.textContent = glyph;
		c.glyph.classList.toggle('hidden', !glyph);
		c.glyph.title = 'Computer player' + (s.n ? ' (' + s.n + ')' : '');
		c.title.textContent = s.t || '';
		c.title.title = s.tn || s.t || '';
		c.title.classList.toggle('hidden', !s.t);
		c.name.textContent = label;
		// the name ellipsizes at these widths; hovering it spells it out
		c.name.title = label;
		// deploy phase: the clock slot carries the seat's confirmation instead
		c.time.classList.toggle('hidden', !!deploying);
		c.lock.classList.toggle('hidden', !deploying);
		c.lock.textContent = s.lk ? '✓' : '⋯';
		c.lock.classList.toggle('locked', !!s.lk);
		c.lock.title = s.lk ? 'Deployment confirmed' : 'Still arranging pieces';
	};

	const createSlot = (g) => {
		const card = document.createElement('a');
		card.className = 'tv-card';
		card.href = '/' + g.r;
		card.dataset.room = g.r;

		// the two seat strips are fixed in place (top above the board, bottom
		// below); updateSlot maps each color to a strip from g.or so the anchored
		// player keeps the bottom seat while the board flips between games
		const top = clockEl(false);
		const bottom = clockEl(true);

		const board = document.createElement('div');
		board.className = 'tv-board gcon';
		const gwrap = document.createElement('div');
		// board theme + piece set come from the [data-board]/[data-piece]
		// attributes on <html> (set by the no-flash script + preferences popover)
		gwrap.className = 'gwrap';
		const ogWrap = document.createElement('div');
		ogWrap.className = 'og-wrap';
		gwrap.appendChild(ogWrap);
		// pre-game overlays, layered over the board exactly as in the room: the
		// two home-rank "?" covers for the blind deploy, then the countdown dial
		const dqTop = questionBand(false);
		const dqBtm = questionBand(true);
		const dial = dialEl();
		// and the post-game result, which takes the dial's place once a game ends
		const result = resultEl();
		gwrap.appendChild(dqTop);
		gwrap.appendChild(dqBtm);
		gwrap.appendChild(dial.root);
		gwrap.appendChild(result.root);
		board.appendChild(gwrap);

		// player names, per-side score and clocks all live on the two seat strips,
		// so the caption is just the variant (time control + game mode) plus the
		// spectator count at the row's end
		const info = document.createElement('div');
		info.className = 'tv-info';
		const variantEl = document.createElement('span');
		variantEl.className = 'tv-variant';
		info.appendChild(variantEl);
		// match length, right of the time control: it qualifies the two score
		// chips, which otherwise read as a one-off game's result
		const raceEl = document.createElement('span');
		raceEl.className = 'tv-race hidden';
		info.appendChild(raceEl);
		const watch = document.createElement('span');
		watch.className = 'tv-watch hidden';
		watch.title = 'Spectators watching';
		watch.innerHTML = EYE_ICON;
		const watchCount = document.createElement('span');
		watch.appendChild(watchCount);
		info.appendChild(watch);

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
			card, og, top, bottom, variantEl, raceEl, watch, watchCount, dqTop, dqBtm, dial, result,
			// whiteEl/blackEl: which fixed row currently holds each color; remapped
			// by updateSlot as the anchored side flips between games
			whiteEl: orient === 'w' ? bottom : top,
			blackEl: orient === 'w' ? top : bottom,
			control: g.tc, wt: g.w, bt: g.b, casual: !!g.ca, toMove: 'w', at: Date.now(),
			over: false, running: false, orient: orient,
			// pre-game phase: whether the arrangements are still hidden, and the
			// local deadline (ms) + span the dial counts down over
			deploying: false, phaseEnd: 0, phaseTotalMs: 0,
			// gameId, plus the id of the game whose result has already been
			// posted: together they fire the end-of-game flash and overlay
			// exactly once per game, and reset when the next game takes the slot
			gameId: g.i, ended: '', resultTimer: null
		};
	};

	const updateSlot = (slot, g) => {
		// anchored color sits in the bottom row; remap each color to its fixed row
		// so the anchored player keeps the bottom seat while the board itself flips
		slot.orient = orientOf(g);
		slot.whiteEl = slot.orient === 'w' ? slot.bottom : slot.top;
		slot.blackEl = slot.orient === 'w' ? slot.top : slot.bottom;

		// a new game id in this slot (rematch/backfill/next game of a match) →
		// drop the finished game's overlay and flash, whether or not they have
		// run their course; they belong to a game that is no longer on the board
		if (g.i && g.i !== slot.gameId) {
			slot.gameId = g.i;
			slot.ended = '';
			hideResult(slot);
			clearScoreFlash(slot.top);
			clearScoreFlash(slot.bottom);
		}

		slot.control = g.tc;
		slot.wt = g.w;
		slot.bt = g.b;
		slot.casual = !!g.ca;
		slot.toMove = sideToMove(g.o);
		slot.at = Date.now();
		slot.over = !!g.x;
		// the server reports a clock as running only while it is actually charging
		// someone — never through the deploy phase or the first-move grace — so the
		// interpolator below can key off this alone
		slot.running = !!g.rn;

		// pre-game phase. The dial's deadline is rebased locally off every update
		// (the server sends what is left, not an absolute time), and the "?" covers
		// go up for the whole blind deploy.
		slot.deploying = !!g.dg;
		const phaseLeftMs = (g.pl || 0) * 10;
		slot.phaseEnd = phaseLeftMs > 0 ? slot.at + phaseLeftMs : 0;
		slot.phaseTotalMs = (g.pt || g.pl || 0) * 10;
		slot.dqTop.classList.toggle('deploy-show', slot.deploying);
		slot.dqBtm.classList.toggle('deploy-show', slot.deploying);
		// paint once now so the dial never shows a frame of the previous phase,
		// then hand it to the animation loop
		if (paintDial(slot, slot.at)) {
			armDials();
		}

		slot.og.set({
			ofen: boardOf(g.o),
			lastMove: lastMoveOf(g.l),
			turnColor: slot.toMove === 'w' ? 'white' : 'black',
			// re-orient on rematch so the board flips to keep the anchored side down
			orientation: slot.orient === 'w' ? 'white' : 'black'
		});

		// caption: time control name (the CSS uppercases it). Every game is the
		// blind-deploy "Octad" mode now, so no mode suffix is shown.
		const caption = g.vn || 'Octad';
		slot.variantEl.textContent = caption;
		// match length, beside the time control. Written in sentence case like
		// every other "Race to N" on the site; the caption row uppercases it.
		const race = (g.rt || 0) > 0 ? 'Race to ' + g.rt : '';
		slot.raceEl.textContent = race;
		slot.raceEl.classList.toggle('hidden', race === '');
		// the overlay is aria-hidden decoration on a link, so a finished game
		// says what it says in the card's own tooltip as well
		slot.card.title = (slot.deploying ? 'Deploying · ' : '')
			+ (slot.over && g.wr ? resultSummary(g) + ' · ' : '')
			+ (g.vb ? 'vs Computer · ' : '') + caption
			+ (race ? ' · ' + race : '');
		setWatchers(slot, g.sp || 0);

		// seat identities go to whichever strip currently holds that color; both
		// are written every update so a flip never leaves a stale name behind
		setSeat(slot.whiteEl, g.ws, slot.deploying);
		setSeat(slot.blackEl, g.bs, slot.deploying);

		// per-side score. A match nobody has scored in yet shows no score at all —
		// a pair of zeroes says nothing, and the room it buys goes to the names,
		// which at the md breakpoint have barely 30px to work with. Both sides are
		// toggled together so the two strips' clocks stay in one column.
		const sw = scoreOf(g, 'w');
		const sb = scoreOf(g, 'b');
		const scored = sw > 0 || sb > 0;
		applyScore(slot.whiteEl, sw, scored);
		applyScore(slot.blackEl, sb, scored);

		slot.card.classList.toggle('over', slot.over);

		// a game just finished on this board: pulse both score chips with the
		// outcome and post the result overlay, once. Keyed off the game id rather
		// than a score delta so that a viewer who arrives during the interlude —
		// with no delta to observe — is told what happened too, instead of being
		// left with an unexplained frozen board.
		if (slot.over && g.i && g.i !== slot.ended && g.wr) {
			slot.ended = g.i;
			flashResult(slot, g.wr);
			showResult(slot, g);
		}

		// the accent "to move" bar means it is someone's turn. Nobody is on move
		// during the blind deploy — both sides act at once — so both bars stay
		// neutral there; the pre-start grace does have a side to move (they may
		// move at any point under the countdown) and keeps its accent.
		const onMove = !slot.over && !slot.deploying;
		paintClock(slot.whiteEl, slot.control, slot.wt, onMove && slot.toMove === 'w', slot.casual);
		paintClock(slot.blackEl, slot.control, slot.bt, onMove && slot.toMove === 'b', slot.casual);
	};

	// setWatchers writes the spectator count; the indicator only renders while
	// someone is actually watching (zero hides it entirely)
	const setWatchers = (slot, n) => {
		slot.watchCount.textContent = n;
		slot.watch.classList.toggle('hidden', n <= 0);
	};

	// applyScore writes a side's score. `shown` gates the whole chip on the match
	// having a score at all.
	const applyScore = (c, value, shown) => {
		c.score.textContent = value;
		c.score.classList.toggle('hidden', !shown);
	};

	// flashResult pulses both score chips with the finished game's outcome: green
	// on the winner, red on the loser, grey on both for a draw. The loser's chip
	// is the point of the pair — it is the only place the card can say who lost,
	// and a match score reads as a result only when both halves of it do.
	const flashResult = (slot, winner) => {
		const draw = winner === 'd';
		flashScore(slot.whiteEl, draw ? 'draw' : (winner === 'w' ? 'win' : 'loss'));
		flashScore(slot.blackEl, draw ? 'draw' : (winner === 'b' ? 'win' : 'loss'));
	};

	const flashScore = (c, kind) => {
		clearScoreFlash(c);
		void c.score.offsetWidth; // reflow so re-adding the class restarts the animation
		c.score.classList.add('score-' + kind);
	};

	const clearScoreFlash = (c) => c.score.classList.remove('score-win', 'score-draw', 'score-loss');

	// showResult posts the finished game's result over the board for
	// resultHoldMs, then fades it. Called once per game (updateSlot gates on the
	// game id), and cut short by hideResult if the next game arrives first.
	const showResult = (slot, g) => {
		const draw = g.wr === 'd';
		slot.result.glyph.innerHTML = RESULT_GLYPHS[g.rs] || '';
		slot.result.glyph.classList.toggle('draw', draw);
		setResultScore(slot.result.white, scoreOf(g, 'w'), draw, g.wr === 'w');
		setResultScore(slot.result.black, scoreOf(g, 'b'), draw, g.wr === 'b');
		slot.result.root.classList.add('on');
		clearTimeout(slot.resultTimer);
		slot.resultTimer = setTimeout(() => {
			slot.result.root.classList.remove('on');
			slot.resultTimer = null;
		}, resultHoldMs);
	};

	const hideResult = (slot) => {
		clearTimeout(slot.resultTimer);
		slot.resultTimer = null;
		slot.result.root.classList.remove('on');
	};

	// setResultScore writes one side's points in the overlay, tinted by how that
	// side finished. The piece-color underbar is static (a class on the element),
	// so only the tint changes per game.
	const setResultScore = (el, value, draw, won) => {
		el.textContent = value;
		el.classList.toggle('draw', draw);
		el.classList.toggle('win', !draw && won);
		el.classList.toggle('loss', !draw && !won);
	};

	// resultSummary spells the outcome out for the card's tooltip, by color
	// rather than by seat — the grid is broadcast to every viewer alike, so
	// there is no "you" to write for (cf. resultSummary in lio-game.js).
	const resultSummary = (g) => {
		const who = g.wr === 'd' ? 'Draw' : (g.wr === 'w' ? 'White wins' : 'Black wins');
		const method = RESULT_METHODS[g.rs] || '';
		return method ? who + ' ' + method : who;
	};

	const paintClock = (c, control, centis, running, casual) => {
		if (casual) {
			// untimed casual game: static ∞, full bar, never "low"
			c.fill.style.width = '100%';
			c.time.textContent = '∞';
			c.root.classList.toggle('run', running);
			c.root.classList.remove('low');
			return;
		}
		centis = Math.max(centis, 0);
		c.fill.style.width = barPct(control, centis);
		c.time.textContent = fmtTime(centis);
		c.root.classList.toggle('run', running);
		c.root.classList.toggle('low', centis < 1000); // < 10s
	};

	// paintDial ticks a slot's pre-game countdown — the whole-seconds number plus
	// the ring sweep — and reports whether that countdown is still live.
	//
	// Lapsing matters differently per phase, and that difference is the whole
	// point of tracking `deploying`:
	//   - the first-move grace lapsing is the instant the server puts the side to
	//     move on the clock, so hand off to the interpolator from the last known
	//     values (the same handoff lio-game.js makes via armWhiteClockTicker) —
	//     no message flows at that moment to arm it for us;
	//   - the deploy timer lapsing only means the auto-fill is in flight, so hold
	//     the covers and the static clocks until the reveal arrives.
	const paintDial = (slot, now) => {
		const remaining = slot.phaseEnd ? slot.phaseEnd - now : 0;
		if (remaining > 0) {
			slot.dial.root.classList.add('on');
			slot.dial.num.textContent = Math.ceil(remaining / 1000);
			const frac = slot.phaseTotalMs > 0 ? Math.min(remaining / slot.phaseTotalMs, 1) : 1;
			// dashoffset runs 0 (full ring) → circumference (empty), as in the room
			slot.dial.progress.style.strokeDashoffset = DIAL_CIRC * (1 - frac);
			return true;
		}
		if (slot.phaseEnd) {
			slot.phaseEnd = 0;
			if (!slot.deploying && !slot.over && !slot.casual) {
				slot.running = true;
				slot.at = now;
			}
		}
		slot.dial.root.classList.remove('on');
		return false;
	};

	// The dials run on their own animation frame rather than the 250ms clock
	// ticker below: at 4 updates a second the sweep visibly staircases, and this
	// is the one element on the card whose whole job is to look like time
	// passing. The loop is self-arming and stops itself the moment no board has a
	// live countdown, so a grid of games under way costs nothing.
	let dialRafId = null;
	const dialFrame = () => {
		const now = Date.now();
		let live = false;
		slots.forEach((slot) => {
			if (paintDial(slot, now)) {
				live = true;
			}
		});
		dialRafId = live ? requestAnimationFrame(dialFrame) : null;
	};
	const armDials = () => {
		if (dialRafId === null) {
			dialRafId = requestAnimationFrame(dialFrame);
		}
	};

	// one shared ticker decrements the active side's clock on every board (one
	// timer, not one per board); the next move delta resets `at` + clocks from
	// the server
	setInterval(() => {
		const now = Date.now();
		slots.forEach((slot) => {
			// don't tick a finished game, one whose clock the server isn't
			// charging yet (deploy phase, pre-start grace, pre-first-move), or an
			// untimed casual game (its ∞ is static)
			if (slot.over || !slot.running || slot.casual) {
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
		if (liveDot) {
			liveDot.classList.toggle('hidden', n === 0);
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

// Pause the home-activity poll while the tab is backgrounded. htmx keeps its
// `every 5s` timer running in hidden tabs (browsers only throttle it), so a
// backgrounded home tab would keep hitting /home/activity indefinitely. htmx's
// own trigger filter (`every 5s [expr]`) can't be used: compiling it needs eval,
// which the site CSP forbids — so gate it here at request time instead. Only
// unattended polling fires while hidden (a user can't click a hidden tab), so
// cancelling any htmx request from #home-activity on document.hidden is safe;
// htmx reschedules the next tick, so polling resumes when the tab is visible.
(function () {
	document.addEventListener('htmx:beforeRequest', function (evt) {
		var elt = evt.detail && evt.detail.elt;
		if (document.hidden && elt && elt.id === 'home-activity') {
			evt.preventDefault();
		}
	});
})();
