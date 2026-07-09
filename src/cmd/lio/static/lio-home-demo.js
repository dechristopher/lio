// lio-home-demo.js — home-page "What is Octad?" self-playing demo board.
//
// octadground only renders positions; move generation and outcome detection live
// server-side (the octad library), so this client fetches a batch of fully
// generated random games from /home/demo and animates them. Each game starts
// from a randomized setup, plays out move-by-move with a 1-3s beat between moves,
// shows a small result pill (the same .end-annotation used in analysis mode) for
// a few seconds on any finish, then rolls into the next game — refetching a fresh
// batch when the current one is exhausted. It self-guards on its mount so it is
// inert on pages without the demo board.
(function () {
	const mount = document.getElementById('home-demo-board');
	const overlay = document.getElementById('home-demo-overlay');
	if (!mount || typeof Octadground === 'undefined') {
		return;
	}

	// timing (ms): a short beat before a game's first move, a brisk random gap
	// between moves that keeps the demo flowing, and a hold on the result pill
	// before the next game begins.
	const firstMoveDelay = 450;
	const minMoveDelay = 600;
	const maxMoveDelay = 1400;
	const resultHold = 5000;
	const refetchRetry = 5000;

	// method code -> result-pill phrasing (mirrors resultReasons in lio-game.js)
	const methods = {
		checkmate: 'by checkmate',
		stalemate: 'by stalemate',
		repetition: 'by repetition',
		moverule: 'by the 25-move rule',
		insufficient: 'by insufficient material',
	};

	const og = Octadground(mount, {
		ofen: 'ppkn/4/4/NKPP',
		orientation: 'white',
		viewOnly: true,
		coordinates: true,
		highlight: {lastMove: true, check: true},
		drawable: {enabled: false},
	});

	// ---- animation state ----
	let games = [];   // current batch
	let gi = 0;       // index of the game being played
	let pi = 0;       // next ply to apply within that game

	// ---- pausable single-shot scheduler ----
	// One timer drives the whole loop; each step arms the next. When the tab is
	// hidden we clear the timer but keep the pending action, then re-arm it (from
	// a full delay — exact remaining time doesn't matter for a demo) once visible
	// again, so a backgrounded tab never animates.
	let timer = null;
	let pending = null; // { fn, delay }
	let paused = document.hidden;

	const arm = (fn, delay) => {
		pending = {fn: fn, delay: delay};
		clearTimeout(timer);
		if (paused) {
			return;
		}
		timer = setTimeout(() => {
			const p = pending;
			pending = null;
			timer = null;
			if (p && p.fn) {
				p.fn();
			}
		}, delay);
	};

	const moveDelay = () => minMoveDelay + Math.floor(Math.random() * (maxMoveDelay - minMoveDelay + 1));

	// ---- overlay ----
	const showOverlay = (text) => {
		if (!overlay) {
			return;
		}
		overlay.textContent = text;
		overlay.classList.add('ea-show');
	};
	const hideOverlay = () => {
		if (!overlay) {
			return;
		}
		overlay.classList.remove('ea-show');
		overlay.textContent = '';
	};

	const resultText = (game) => {
		const method = methods[game.method] || '';
		if (game.winner === 'd') {
			return method ? ('Draw ' + method) : 'Draw';
		}
		const who = game.winner === 'w' ? 'White wins' : 'Black wins';
		return method ? (who + ' ' + method) : who;
	};

	// ---- board helpers ----
	const boardOf = (ofen) => (ofen || '').split(' ')[0];
	const turnOf = (ofen) => ((ofen || '').split(' ')[1] === 'b' ? 'black' : 'white');
	const lastMoveOf = (uoi) =>
		(uoi && uoi.length >= 4) ? [uoi.substring(0, 2), uoi.substring(2, 4)] : [];

	// ---- game loop ----
	const fetchBatch = () => {
		fetch('/home/demo', {headers: {Accept: 'application/json'}})
			.then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
			.then((list) => {
				if (!Array.isArray(list) || list.length === 0) {
					arm(fetchBatch, refetchRetry);
					return;
				}
				games = list;
				gi = 0;
				playCurrent();
			})
			.catch(() => arm(fetchBatch, refetchRetry));
	};

	const playCurrent = () => {
		if (gi >= games.length) {
			fetchBatch(); // batch exhausted → fresh games
			return;
		}
		const game = games[gi];
		pi = 0;
		hideOverlay();
		og.set({
			ofen: boardOf(game.start),
			orientation: 'white',
			turnColor: turnOf(game.start),
			lastMove: [],
			check: false, // a fresh start position is never in check
		});
		arm(stepMove, firstMoveDelay);
	};

	const stepMove = () => {
		const game = games[gi];
		if (!game || pi >= game.plies.length) {
			finishGame(game);
			return;
		}
		const ply = game.plies[pi++];
		og.set({
			ofen: boardOf(ply.o),
			turnColor: turnOf(ply.o),
			lastMove: lastMoveOf(ply.u),
			check: !!ply.k, // highlight the king when this move leaves it in check
		});
		if (pi >= game.plies.length) {
			// reveal the result the instant the final move lands (no extra beat)
			finishGame(game);
		} else {
			arm(stepMove, moveDelay());
		}
	};

	const finishGame = (game) => {
		if (game) {
			showOverlay(resultText(game));
		}
		arm(() => {
			gi++;
			playCurrent();
		}, resultHold);
	};

	// ---- lifecycle ----
	document.addEventListener('visibilitychange', () => {
		const wasPaused = paused;
		paused = document.hidden;
		if (paused) {
			clearTimeout(timer);
			timer = null;
		} else if (wasPaused && pending) {
			arm(pending.fn, pending.delay);
		}
	});

	window.addEventListener('pagehide', () => {
		paused = true;
		clearTimeout(timer);
		timer = null;
		try {
			og.destroy();
		} catch (e) { /* ignore */ }
	});

	fetchBatch();
})();
