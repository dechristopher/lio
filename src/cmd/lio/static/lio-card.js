// lio-card.js — the username hover card (arch/PLAYER_CARD.md).
//
// Hovering any username on the site opens a small readout of that player: their
// ratings, their overall record, whether they are here, and — when they are at
// a board — a live mini board of the game they are playing.
//
// Three things about the shape of this file:
//
//  1. It binds nothing per username. One delegated pointer listener matches
//     every anchor into /@/, so a surface gets the card by linking a name the
//     way the site already links names, and no new surface has to remember to
//     opt in. A link (or any ancestor) carrying data-nocard opts out.
//
//  2. It is pointer-only. Hover has no touch equivalent worth faking: a tap
//     that opened a card would either eat the navigation or flash and vanish
//     when the tap ended, and the full version of everything here is one tap
//     away on the profile page. Touch pointers are ignored outright. Keyboard
//     focus does open the card, because there the card is additive — Tab still
//     moves and Enter still follows the link.
//
//  3. It holds no socket. Every page already has exactly one (the room's, the
//     home page's, or /socket/me) and its owner hands it over through wire();
//     the card asks the server to stream one room at a time over it. Without a
//     socket the card still works — the fetch carries a full board snapshot,
//     which simply stops updating.
(function () {
	// dwell before a hover opens anything. Long enough that crossing a name on
	// the way somewhere else costs no request, short enough to feel like the
	// card was already there.
	const openDelayMs = 320;
	// grace after the pointer leaves, so the gap between the name and the card
	// is crossable and a jittery pointer does not flicker it.
	const closeDelayMs = 200;
	// how long a fetched card stays good. Reading down a roster re-crosses the
	// same names constantly; the live half of the card comes over the socket
	// anyway, so the cached half is the slow-moving half.
	const cacheTTLms = 60000;
	// horizontal room to leave against the viewport edge when placing the card
	const edgePad = 8;
	// gap between the anchor and the card
	const anchorGap = 6;

	const cache = new Map(); // username (lowercased) -> {at, data}

	let root = null;      // the card element, built on first use
	let parts = null;     // its inner handles
	let board = null;     // the miniboard handle, while a game is shown
	let anchorEl = null;  // the link the open card belongs to
	let openName = '';    // username the open card describes
	let openTimer = null;
	let closeTimer = null;
	let send = null;      // socket sender, handed over by the page's owner
	let watching = '';    // room id the server is streaming to us

	const mini = () => window.lioMiniboard || null;

	// ---- the element ------------------------------------------------------

	const make = (tag, cls) => {
		const el = document.createElement(tag);
		if (cls) {
			el.className = cls;
		}
		return el;
	};

	// build assembles the card once and keeps it. It ships hidden and empty, so
	// nothing here can move the page: it is fixed-position and only ever
	// measured against a link's own rect.
	const build = () => {
		root = make('div', 'pcard');
		root.hidden = true;
		root.setAttribute('role', 'tooltip');

		const head = make('div', 'pcard-head');
		const name = make('a', 'pcard-name');
		const title = make('span', 'player-title pcard-title');
		title.hidden = true;
		const nameText = make('span', 'pcard-name-text');
		name.appendChild(title);
		name.appendChild(nameText);
		const status = make('span', 'pcard-status');
		const dot = make('i', 'pcard-dot');
		const statusText = make('span');
		status.appendChild(dot);
		status.appendChild(statusText);
		head.appendChild(name);
		head.appendChild(status);

		const ratings = make('div', 'pcard-ratings');
		const record = make('div', 'pcard-record');
		// the board's slot. It is sized by CSS from the moment a game is known
		// to exist — before the renderer has even loaded — so the card never
		// grows under the pointer once it is on screen.
		const game = make('div', 'pcard-game');
		game.hidden = true;
		const foot = make('div', 'pcard-foot');

		root.appendChild(head);
		root.appendChild(ratings);
		root.appendChild(record);
		root.appendChild(game);
		root.appendChild(foot);
		document.body.appendChild(root);

		// the pointer moving onto the card keeps it open; leaving it closes on
		// the same grace as leaving the link
		root.addEventListener('pointerenter', () => clearTimeout(closeTimer));
		root.addEventListener('pointerleave', scheduleClose);

		parts = {name, title, nameText, status, dot, statusText, ratings, record, game, foot};
	};

	// ---- rendering --------------------------------------------------------

	const setText = (el, text) => {
		el.textContent = text || '';
	};

	// render writes one card payload. Everything is written as text — a username,
	// a title code and a status line are all account data.
	const render = (d) => {
		parts.name.href = d.url || '#';
		setText(parts.nameText, d.username);
		parts.title.hidden = !d.title;
		setText(parts.title, d.title || '');
		parts.title.title = d.titleName || d.title || '';

		setText(parts.statusText, d.status || '');
		parts.dot.className = 'pcard-dot'
			+ (d.playing ? ' playing' : (d.online ? ' online' : ''));
		parts.status.hidden = !d.status;

		renderRatings(d.ratings);
		renderRecord(d.record);
		setText(parts.foot, d.joined ? 'Member since ' + d.joined : '');
		parts.foot.hidden = !d.joined;
	};

	const renderRatings = (rows) => {
		parts.ratings.textContent = '';
		if (!rows || !rows.length) {
			parts.ratings.hidden = true;
			return;
		}
		parts.ratings.hidden = false;
		rows.forEach((r) => {
			const tile = make('div', 'pcard-rating');
			const label = make('span', 'pcard-rating-label');
			// the time control, not the speed class: two of the four categories
			// are rapid, so a row of speeds can label two tiles identically and
			// say nothing about either. The speed rides the tooltip with it.
			setText(label, r.label || r.speed);
			const value = make('span', 'pcard-rating-value');
			setText(value, r.rating);
			tile.appendChild(value);
			tile.appendChild(label);
			tile.title = r.label + (r.speed ? ' ' + r.speed : '') + ' · '
				+ r.games + (r.games === 1 ? ' game' : ' games');
			parts.ratings.appendChild(tile);
		});
	};

	const renderRecord = (rec) => {
		parts.record.textContent = '';
		if (!rec || !rec.games) {
			parts.record.hidden = true;
			return;
		}
		parts.record.hidden = false;
		const bar = make('div', 'pcard-bar');
		const total = rec.games || 1;
		[['win', rec.wins], ['draw', rec.draws], ['loss', rec.losses]].forEach((pair) => {
			if (!pair[1]) {
				return;
			}
			const seg = make('i', 'pcard-bar-' + pair[0]);
			seg.style.width = ((pair[1] / total) * 100) + '%';
			bar.appendChild(seg);
		});
		const legend = make('div', 'pcard-legend');
		const counts = make('span', 'pcard-counts');
		// built in Go-free pieces so each number can carry its own color; templ's
		// text-node trimming problem does not apply here, but the three-color
		// reading does
		[['win', rec.wins], ['draw', rec.draws], ['loss', rec.losses]].forEach((pair, i) => {
			if (i) {
				counts.appendChild(document.createTextNode(' / '));
			}
			const n = make('b', 'pcard-' + pair[0]);
			setText(n, pair[1]);
			counts.appendChild(n);
		});
		const games = make('span', 'pcard-games');
		setText(games, rec.games + (rec.games === 1 ? ' game' : ' games'));
		legend.appendChild(counts);
		legend.appendChild(games);
		parts.record.appendChild(bar);
		parts.record.appendChild(legend);
		parts.record.title = rec.wins + ' won, ' + rec.draws + ' drawn, ' + rec.losses + ' lost';
	};

	// ---- the live board ---------------------------------------------------

	// viewingRoom reports that the reader is already looking at this room — the
	// room page and its archive both live at /<room id>. The card then leaves the
	// board out: the game is on the screen behind the card at full size, and a
	// thumbnail of what you are already watching is noise.
	const viewingRoom = (roomID) => {
		if (!roomID) {
			return false;
		}
		return location.pathname.split('/')[1] === roomID;
	};

	// showGame mounts (or updates) the mini board for a game payload, and calls
	// back once the card has reached the size it will keep.
	//
	// The callback is the whole reason this is not fire-and-forget. The board is
	// most of the card's height and the renderer may not be loaded yet, so a
	// card revealed before it lands is placed at the wrong height and then grows
	// — off the bottom of the viewport, in the case that first showed it up.
	// Everything else is written synchronously, so waiting on this one thing is
	// what lets the card be revealed exactly once, at its final size.
	const showGame = (g, done) => {
		const settle = done || (() => {});
		if (!g || !mini()) {
			settle();
			return;
		}
		parts.game.hidden = false;
		if (board) {
			mini().update(board, g);
			settle();
			return;
		}
		const forName = openName;
		mini().ensure().then((ok) => {
			// the pointer has moved on, or moved to somebody else, while the
			// board renderer was in flight
			if (openName !== forName) {
				return;
			}
			if (ok && !board) {
				board = mini().create(g, {className: 'pcard-mini'});
				parts.game.appendChild(board.el);
				mini().update(board, g);
			} else if (!ok) {
				// no renderer: take the empty slot back out rather than leave a
				// reserved gap for a board that is never coming
				parts.game.hidden = true;
			}
			settle();
		});
	};

	const clearGame = () => {
		if (board && mini()) {
			mini().destroy(board);
		}
		if (board && board.el) {
			board.el.remove();
		}
		board = null;
		parts.game.textContent = '';
		parts.game.hidden = true;
	};

	// watch asks the page's socket to stream one room to us, replacing whatever
	// it was streaming before. A page with no socket keeps the static snapshot.
	const watch = (roomID) => {
		if (watching === roomID) {
			return;
		}
		watching = roomID;
		if (send) {
			send({t: 'wg', d: {r: roomID}});
		}
	};

	// ---- open / close -----------------------------------------------------

	const isOpen = () => root && !root.hidden;

	const usernameOf = (a) => {
		// same-origin path links only; an absolute URL to another host that
		// happened to contain /@/ is not one of ours
		const href = a.getAttribute('href') || '';
		if (href.indexOf('/@/') !== 0) {
			return '';
		}
		const rest = href.slice(3).split(/[/?#]/)[0];
		try {
			return decodeURIComponent(rest);
		} catch (e) {
			return rest;
		}
	};

	const open = (anchor, username) => {
		anchorEl = anchor;
		openName = username;
		const key = username.toLowerCase();
		const hit = cache.get(key);
		if (hit && (Date.now() - hit.at) < cacheTTLms) {
			show(anchor, hit.data);
			return;
		}
		fetch('/api/card/' + encodeURIComponent(username), {
			credentials: 'same-origin',
			headers: {Accept: 'application/json'}
		}).then((r) => (r.ok ? r.json() : null)).then((d) => {
			if (!d) {
				return;
			}
			cache.set(key, {at: Date.now(), data: d});
			// the pointer may have left, or moved to another name, while this
			// was in flight
			if (openName === username && anchorEl === anchor) {
				show(anchor, d);
			}
		}).catch(() => { /* a card that fails to load simply does not appear */ });
	};

	const show = (anchor, d) => {
		if (!root) {
			build();
		}
		clearGame();
		render(d);
		// a live game the reader is not already watching gets a board, kept
		// current by the socket for as long as the card is open
		if (d.game && d.room && !viewingRoom(d.room)) {
			// measured off-screen: hidden would give it no box to measure, and
			// visible would show the reader a card being assembled
			root.style.visibility = 'hidden';
			root.hidden = false;
			showGame(d.game, () => reveal(anchor));
			watch(d.room);
			return;
		}
		watch('');
		reveal(anchor);
	};

	// reveal shows the card and pins it, in that order — place measures it, so
	// it has to have a box by then.
	const reveal = (anchor) => {
		root.hidden = false;
		root.style.visibility = '';
		place(anchor);
	};

	// place pins the card to its link: below by default, above when the space
	// below cannot hold it, and clamped inside the viewport on both axes. Fixed
	// positioning means these are viewport coordinates already.
	//
	// The final clamp is not redundant with the flip. A card carrying a board is
	// tall enough that on a short window neither side fits, and without it the
	// overflowing end is simply unreachable — the reader cannot scroll a
	// fixed-position element into view.
	const place = (anchor) => {
		const r = anchor.getBoundingClientRect();
		const w = root.offsetWidth;
		const h = root.offsetHeight;

		let left = r.left;
		if (left + w > window.innerWidth - edgePad) {
			left = window.innerWidth - edgePad - w;
		}
		root.style.left = Math.max(edgePad, left) + 'px';

		const below = r.bottom + anchorGap;
		let top = below;
		if (below + h > window.innerHeight - edgePad && r.top - anchorGap - h > edgePad) {
			top = r.top - anchorGap - h;
		}
		root.style.top = Math.max(edgePad,
			Math.min(top, window.innerHeight - edgePad - h)) + 'px';
	};

	const close = () => {
		clearTimeout(openTimer);
		clearTimeout(closeTimer);
		openName = '';
		anchorEl = null;
		watch('');
		if (!root) {
			return;
		}
		clearGame();
		root.hidden = true;
		// a card that was still being measured when the pointer left
		root.style.visibility = '';
	};

	const scheduleClose = () => {
		clearTimeout(closeTimer);
		closeTimer = setTimeout(close, closeDelayMs);
	};

	// ---- events -----------------------------------------------------------

	// anchorFor resolves the hover target to a username link the card should
	// answer for, or null.
	const anchorFor = (target) => {
		if (!target || !target.closest) {
			return null;
		}
		const a = target.closest('a[href^="/@/"]');
		if (!a || a.closest('[data-nocard]')) {
			return null;
		}
		// links inside the card itself: the pointer is on the card, which its
		// own pointerenter already handles
		if (root && root.contains(a)) {
			return null;
		}
		return a;
	};

	document.addEventListener('pointerover', (e) => {
		// touch has no hover — see the note at the top of this file
		if (e.pointerType === 'touch') {
			return;
		}
		const a = anchorFor(e.target);
		if (!a) {
			return;
		}
		clearTimeout(closeTimer);
		if (a === anchorEl && isOpen()) {
			return;
		}
		const username = usernameOf(a);
		if (!username) {
			return;
		}
		clearTimeout(openTimer);
		openTimer = setTimeout(() => open(a, username), openDelayMs);
	});

	document.addEventListener('pointerout', (e) => {
		if (e.pointerType === 'touch') {
			return;
		}
		if (!anchorFor(e.target)) {
			return;
		}
		clearTimeout(openTimer);
		scheduleClose();
	});

	// keyboard: tabbing onto a name opens its card, tabbing off closes it. No
	// dwell — a focus is already deliberate.
	document.addEventListener('focusin', (e) => {
		const a = anchorFor(e.target);
		if (!a) {
			return;
		}
		const username = usernameOf(a);
		if (username) {
			clearTimeout(closeTimer);
			open(a, username);
		}
	});
	document.addEventListener('focusout', (e) => {
		if (anchorFor(e.target)) {
			scheduleClose();
		}
	});
	document.addEventListener('keydown', (e) => {
		if (e.key === 'Escape' && isOpen()) {
			close();
		}
	});

	// the card is pinned to a link's rect, so anything that moves that link
	// re-pins it. Scroll is capture-phase so it catches scrolling containers,
	// not just the document.
	const repin = () => {
		if (isOpen() && anchorEl) {
			place(anchorEl);
		}
	};
	window.addEventListener('scroll', repin, true);
	window.addEventListener('resize', repin);

	// a click anywhere is a navigation or a decision; either way the card has
	// served its purpose
	document.addEventListener('click', (e) => {
		if (!root || !root.contains(e.target)) {
			close();
		}
	});

	// back/forward cache restores the DOM as it was, open card included
	window.addEventListener('pageshow', (e) => {
		if (e.persisted) {
			close();
		}
	});
	window.addEventListener('pagehide', close);

	window.lioCard = {
		// wire hands the card the page socket's sender (null when it drops). The
		// standing watch is re-sent on a reconnect, so a card left open across a
		// blip resumes streaming.
		wire: (fn) => {
			send = fn;
			if (send && watching) {
				send({t: 'wg', d: {r: watching}});
			}
		},
		// live applies a watched room's state, pushed by the server.
		live: (d) => {
			if (!d || !d.r || d.r !== watching || !isOpen()) {
				return;
			}
			if (d.x) {
				// the room ended: drop the board rather than leave a frozen
				// position claiming to be a live game
				clearGame();
				watching = '';
				return;
			}
			if (d.g) {
				showGame(d.g);
			}
		},
	};
})();
