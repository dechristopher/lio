// LIO game handling code
let frameId, frameTime, wt, bt, move = 1;

// Outbound move reliability. Octadground moves are optimistic and the wire
// protocol has no move ACK, so a move lost on a half-open or reconnecting
// socket would vanish silently (the piece stays put, the server never sees it).
// We hold the last unconfirmed move and reconcile it against authoritative
// server state — on an ACK timeout and on every reconnect — resending it if the
// server never received it.
let pendingMove = null;   // { uoi, ply, attempts }; ply = server ply before it
let pendingTimer = null;  // ACK timeout -> reconcilePending
let reconciling = false;  // an authoritative re-query is in flight
let lastPly = 0;          // ply of the latest authoritative state we applied
const ackTimeoutMs = 2500;
const maxReconcileAttempts = 3;

const moveTag = "m";
const gameOverTag = "g";

window.addEventListener('load', () => {
	if (window.ws) {
		return false;
	}
	connect();
	return true;
});


window.moveSound = new Howl({
	src: ["/res/sfx/move.ogg"],
	preload: true,
	autoplay: true,
	html5: true,
	volume: 0.75
});

window.capSound = new Howl({
	src: ["/res/sfx/capture.ogg"],
	preload: true,
	volume: 0.9
});

window.checkSound = new Howl({
	src: ["/res/sfx/check.ogg"],
	preload: true,
	volume: 0.9
});

// create game board
let og = Octadground(document.getElementById('game'), {
	ofen: 'ppkn/4/4/NKPP', // set initial board state to prevent brief period of missing pieces
	orientation: document.getElementById('gcon-xx').classList.contains('w') ? 'white' : 'black',
	highlight: {
		lastMove: true,
		check: true,
	},
	movable: {
		free: false,
		color: document.getElementById('gcon-xx').classList.contains('w') ? 'white' : 'black'
	},
	selectable: {
		enabled: window.isMobile,
	},
	events: {
		move: (orig, dest, capturedPiece) => {
			doMove(orig, dest);
		}
	}
});

/**
 * Number of half-moves reflected in a board-state message — the authoritative
 * ply, used to detect stale snapshots and to confirm/reconcile our own moves.
 * @param message - board-state (move) message
 */
const messagePly = (message) => (message.d.m ? message.d.m.length : 0);

/**
 * Clear the tracked unconfirmed move and stop its reconcile timer.
 */
const clearPending = () => {
	pendingMove = null;
	reconciling = false;
	if (pendingTimer !== null) {
		clearTimeout(pendingTimer);
		pendingTimer = null;
	}
};

/**
 * (Re)arm the ACK timeout that kicks off reconciliation if the server never
 * confirms the pending move.
 */
const armPendingTimer = () => {
	if (pendingTimer !== null) {
		clearTimeout(pendingTimer);
	}
	pendingTimer = setTimeout(reconcilePending, ackTimeoutMs);
};

/**
 * Reconcile an unconfirmed move by re-requesting the authoritative position.
 * handleMove resolves it: confirming the move if it landed, or resending it if
 * the server never received it. Re-arms so a lost query is retried.
 */
const reconcilePending = () => {
	if (!pendingMove) {
		return;
	}
	reconciling = true;
	sendBoardUpdateRequest();
	armPendingTimer();
};

/**
 * Put a move on the wire. Returns whether it was actually sent (false if the
 * socket was down — the move stays pending and is flushed on reconnect).
 * @param uoi - UOI move string
 * @param num - move number
 */
const sendMoveOnWire = (uoi, num) => send(buildCommand("m", {u: uoi, a: num}));

/**
 * Resend the pending move after reconciliation shows the server never got it.
 * Caps attempts so a persistently-rejected move can't spin.
 */
const resendPending = () => {
	if (!pendingMove) {
		return;
	}
	if (pendingMove.attempts >= maxReconcileAttempts) {
		// give up resending and trust whatever authoritative state we have
		clearPending();
		return;
	}
	pendingMove.attempts++;
	sendMoveOnWire(pendingMove.uoi, move);
	armPendingTimer();
};

/**
 * Sends a game move in Universal Octad Interface format. The move is retained
 * as "pending" until the server confirms it (see handleMove), so it survives a
 * failed send on a half-open or reconnecting socket.
 * @param uoi - UOI move string
 * @param num - move number
 */
const sendGameMove = (uoi, num) => {
	pendingMove = {uoi: uoi, ply: lastPly, attempts: 0};
	armPendingTimer();
	sendMoveOnWire(uoi, num);
};

// Invoked by the core client on every (re)connect, just before it re-requests
// board state. If a move is still unconfirmed, flag the imminent board-state
// response as reconciliation so a move lost to the dropped socket is resent.
window.onSocketReconnect = () => {
	if (pendingMove) {
		reconciling = true;
	}
};

/**
 * Returns true if the player is playing white
 * @param message - move message
 * @returns {boolean} is white
 */
const isPlayerWhite = (message) => {
	return message.d.w === getCookie('uid');
};

/**
 * Returns true if it is currently the player's turn
 * @param message
 * @param ofenParts
 * @returns {boolean} is currently player's turn
 */
const isPlayerTurn = (message, ofenParts) => {
	return (isPlayerWhite(message) && whiteToMove(ofenParts))
		|| (!isPlayerWhite(message) && !whiteToMove(ofenParts));
};

/**
 * Returns true if the player is playing white
 * @param ofenParts - split OFEN
 * @returns {boolean} is white's turn
 */
const whiteToMove = (ofenParts) => {
	return ofenParts[1] === 'w';
};

// whether the opponent in this game is the engine/bot, and the opponent's
// "thinking" indicator element (only the opponent clock carries data-bot)
const clockOpponentEl = document.getElementById('clockOpponent');
const opponentIsBot = !!clockOpponentEl && clockOpponentEl.dataset.bot === 'true';
const thinkingEl = clockOpponentEl ? clockOpponentEl.querySelector('.thinking') : null;

/**
 * Show or hide the opponent "thinking" indicator. No-op unless the opponent
 * is the engine, so it never appears in human-vs-human games.
 * @param on - whether the engine is currently thinking
 */
const setThinking = (on) => {
	if (!thinkingEl || !opponentIsBot) {
		return;
	}
	thinkingEl.classList.toggle('thinking-on', !!on);
};

/**
 * Update the thinking indicator from a board-state message: the engine is
 * thinking whenever the game is still ongoing and it is the opponent's turn.
 * @param message - move message
 * @param ofenParts - OFEN parts array
 */
const updateThinking = (message, ofenParts) => {
	// a non-empty legal-move set means the game is still in progress; an empty
	// set means checkmate/stalemate, where nobody is "thinking"
	const gameOngoing = !!message.d.v && Object.keys(message.d.v).length > 0;
	setThinking(gameOngoing && !isPlayerTurn(message, ofenParts));
};

// game-end result overlay elements and the player's color, cached from move
// messages (the game-over message reuses the `w` key for the winner, so the
// player's color can't be derived from it)
const resultOverlayEl = document.getElementById('result-overlay');
const resultHeadlineEl = document.getElementById('result-headline');
const resultReasonEl = document.getElementById('result-reason');
const resultScoreEl = document.getElementById('result-score');
const resultCountdownEl = document.getElementById('result-countdown');
const rematchBtn = document.getElementById('result-rematch');
const homeBtn = document.getElementById('result-home');
let playerWhite = false;
let countdownInterval = null;

// backend reason codes -> human-readable method subtitles
const resultReasons = {
	checkmate: 'by checkmate',
	time: 'on time',
	resignation: 'by resignation',
	stalemate: 'by stalemate',
	insufficient: 'insufficient material',
	agreement: 'by agreement',
	repetition: 'by repetition',
	moverule: 'by 25-move rule',
	abandoned: 'opponent left',
};

/**
 * Stop and clear the auto-rematch countdown.
 */
const stopCountdown = () => {
	if (countdownInterval !== null) {
		clearInterval(countdownInterval);
		countdownInterval = null;
	}
	if (resultCountdownEl) {
		resultCountdownEl.innerHTML = '';
	}
};

/**
 * Start the auto-rematch countdown shown under the result actions. Bot games
 * auto-rematch after a fixed delay; this ticks it down so the player sees how
 * long they have to decide. The new-game broadcast clears it via hideResult.
 * @param seconds - whole seconds until the auto-rematch fires
 */
const startAutoRematchCountdown = (seconds) => {
	stopCountdown();
	if (!resultCountdownEl || !seconds || seconds <= 0) {
		return;
	}

	let remaining = seconds;
	const render = () => {
		resultCountdownEl.innerHTML = remaining > 0
			? `New game in ${remaining}&hellip;`
			: 'Starting new game&hellip;';
	};
	render();

	countdownInterval = setInterval(() => {
		remaining -= 1;
		render();
		if (remaining <= 0) {
			clearInterval(countdownInterval);
			countdownInterval = null;
		}
	}, 1000);
};

/**
 * Hide the game-end result overlay and reset its rematch button and countdown.
 */
const hideResult = () => {
	if (resultOverlayEl) {
		resultOverlayEl.classList.remove('result-show');
	}
	if (rematchBtn) {
		rematchBtn.disabled = false;
		rematchBtn.innerHTML = 'Rematch';
	}
	stopCountdown();
};

/**
 * Populate and show the game-end result overlay from a game-over message.
 * @param message - game over message
 */
const showResult = (message) => {
	if (!resultOverlayEl) {
		return;
	}

	const winner = message.d.w; // "w", "b", or "d"

	let outcome, headline;
	if (message.d.r === 'abandoned') {
		// abandonment closes the room; report it neutrally rather than as a draw
		outcome = 'draw';
		headline = 'Match over';
	} else if (winner === 'd') {
		outcome = 'draw';
		headline = 'Draw';
	} else if ((winner === 'w' && playerWhite) || (winner === 'b' && !playerWhite)) {
		outcome = 'win';
		headline = 'You win';
	} else {
		outcome = 'loss';
		headline = 'You lose';
	}

	// a rematch is impossible once the whole room is over (abandon / match end)
	if (rematchBtn) {
		rematchBtn.style.display = message.d.o ? 'none' : '';
	}

	resultHeadlineEl.className = `result-headline ${outcome}`;
	resultHeadlineEl.innerHTML = headline;

	// method subtitle: prefer the structured reason code, falling back to the
	// full status string the footer already shows
	resultReasonEl.innerHTML = resultReasons[message.d.r] || message.d.s || '';

	// match score, player's score first
	if (message.d.sc) {
		const mine = playerWhite ? message.d.sc.w : message.d.sc.b;
		const theirs = playerWhite ? message.d.sc.b : message.d.sc.w;
		resultScoreEl.innerHTML = `${mine} &ndash; ${theirs}`;
	} else {
		resultScoreEl.innerHTML = '';
	}

	// bot games auto-rematch; show the countdown so the player can leave or
	// rematch before the next game starts
	startAutoRematchCountdown(message.d.ar);

	resultOverlayEl.classList.add('result-show');
};

// wire result overlay actions once; the new-game broadcast clears the overlay
if (rematchBtn) {
	rematchBtn.addEventListener('click', () => {
		send(buildCommand("r", {rm: true}));
		// leave the overlay up until the rematch actually starts, but signal
		// that the request was sent and stop any auto-rematch countdown
		rematchBtn.disabled = true;
		rematchBtn.innerHTML = 'Waiting&hellip;';
		stopCountdown();
	});
}
if (homeBtn) {
	homeBtn.addEventListener('click', () => {
		window.location.href = "/";
	});
}

/**
 * Handle incoming move messages, update board state, update UI and clocks
 * @param message - move message
 */
const handleMove = (message) => {
	const ofenParts = message.d.o.split(' ');
	const serverPly = messagePly(message);

	// ignore a stale board snapshot that would regress the board to an older
	// position (e.g. a late board-state response landing after newer state). A
	// game-start/reset (gs) legitimately resets the ply, so always honor it.
	if (!message.d.gs && serverPly < lastPly) {
		return;
	}

	// reconcile any move we sent but haven't seen confirmed yet
	if (pendingMove) {
		if (serverPly > pendingMove.ply) {
			// the server advanced past our move: it landed — confirmed
			clearPending();
		} else if (reconciling) {
			// we explicitly re-queried and the server is still at our pre-move
			// position, so the move never arrived. Resend if it's still our turn.
			reconciling = false;
			if (isPlayerTurn(message, ofenParts)) {
				resendPending();
			} else {
				clearPending();
			}
		}
	}

	lastPly = serverPly;

	if (!message.d.m) {
		move = 1;
		document.getElementById("info").innerHTML = "";
	}

	// cache the player's color for the result overlay, and clear any prior
	// result when a new game (e.g. a rematch) starts
	playerWhite = isPlayerWhite(message);
	if (message.d.gs) {
		hideResult();
		// a new game invalidates any move left unconfirmed from the prior one
		clearPending();
	}

	// play sounds
	playSounds(message, ofenParts);

	og.set({
		orientation: message.d.w === getCookie('uid') ? 'white' : 'black',
		ofen: ofenParts[0],
		lastMove: getLastMove(message.d.m),
		turnColor: whiteToMove(ofenParts) ? "white" : "black",
		check: message.d.k,
		movable: {
			free: false,
			dests: allMoves(message.d.v),
			color: message.d.w === getCookie('uid') ? 'white' : 'black',
		}
	});

	// update UI styles and clock tickers
	updateUI(message, ofenParts);

	// show/hide the engine thinking indicator based on whose turn it is
	updateThinking(message, ofenParts);

	// perform pre-move if set
	og.playPremove();
};

/**
 * Handles incoming game over messages
 * @param message - game over message
 */
const handleGameOver = (message) => {
	cancelAnimationFrame(frameId);
	document.getElementById("info").innerHTML = message.d.s;
	window.notification.play();

	// game is over; the engine is no longer thinking
	setThinking(false);

	// surface the outcome prominently over the board, but only when the message
	// carries an actual result; bare room-closing notices (no winner) just
	// trigger the redirect below and leave any existing result card in place
	if (message.d.w) {
		showResult(message);
	}

	// disallow further moves
	og.set({
		movable: {
			dests: new Map()
		}
	})

	// update match score
	updateScore(message);

	// if room over, redirect home after a second
	if (message.d.o === true) {
		setTimeout(() => {
			window.location.href = "/";
		}, 3000);
	}
};


/**
 * playSounds will play confirmation and move sounds depending
 * on the contents of the move message received
 * @param message - move message
 * @param ofenParts - OFEN parts array
 */
const playSounds = (message, ofenParts) => {
	if (message.d.gs) {
		// play confirmation sound on game start
		window.confirmation.play();
	} else {
		// play move sounds if game is not starting
		// and only if board ofen is different from current
		if (message.d.s && ofenParts[0] !== og.state.ofen) {
			playMoveSound(message);
		}
	}
};

/**
 * Update the match score using the given message
 * @param message - move/game-over message
 */
const updateScore = (message) => {
	if (!message.d.sc) {
		return;
	}

	const plyClock = document.getElementById("clockPlayer");
	const oppClock = document.getElementById("clockOpponent");

	const plyScore = plyClock.getElementsByClassName("clockScore")[0];
	const oppScore = oppClock.getElementsByClassName("clockScore")[0];

	if (isPlayerWhite(message)) {
		plyScore.innerHTML = message.d.sc.w;
		oppScore.innerHTML = message.d.sc.b;
	} else {
		plyScore.innerHTML = message.d.sc.b;
		oppScore.innerHTML = message.d.sc.w;
	}
}

/**
 * updateUI updates UI state, styles and clock tickers
 * @param message - move message
 * @param ofenParts - OFEN parts array
 */
const updateUI = (message, ofenParts) => {
	cancelAnimationFrame(frameId);

	wt = message.d.c.w;
	bt = message.d.c.b;

	// update match score
	updateScore(message);

	const plyClock = document.getElementById("clockPlayer");
	const oppClock = document.getElementById("clockOpponent");

	const plyTime = plyClock.getElementsByClassName("clockTime")[0];
	const oppTime = oppClock.getElementsByClassName("clockTime")[0];

	let playerTimeRemaining = isPlayerWhite(message) ? wt : bt;
	let opponentTimeRemaining = isPlayerWhite(message) ? bt : wt;

	// set clock times
	plyTime.innerHTML = timeFormatter(playerTimeRemaining);
	oppTime.innerHTML = timeFormatter(opponentTimeRemaining);

	// low-time emphasis (<10s = 1000 centiseconds): toggled on the clock wrapper
	// so the time + progress bar shift to the loss color and pulse (app.css .low)
	plyClock.classList.toggle('low', playerTimeRemaining < 1000);
	oppClock.classList.toggle('low', opponentTimeRemaining < 1000);

	const plyBar = plyClock.getElementsByClassName("clockProgressBar")[0];
	const oppBar = oppClock.getElementsByClassName("clockProgressBar")[0];

	// set time bar progress
	plyBar.style.width = barWidth(message.d.c.tc, playerTimeRemaining);
	oppBar.style.width = barWidth(message.d.c.tc, opponentTimeRemaining);

	// set clock UI active state
	if (isPlayerTurn(message, ofenParts)) {
		plyClock.classList.add('active');
		oppClock.classList.remove('active');
	} else {
		plyClock.classList.remove('active');
		oppClock.classList.add('active');
	}

	// set player name colors
	if (isPlayerWhite(message)) {
		oppClock.classList.add('playerBlack');
		oppClock.classList.remove('playerWhite');
		plyClock.classList.add('playerWhite');
		plyClock.classList.remove('playerBlack');
	} else {
		oppClock.classList.add('playerWhite');
		oppClock.classList.remove('playerBlack');
		plyClock.classList.add('playerBlack');
		plyClock.classList.remove('playerWhite');
	}

	// only run this when move is provided, otherwise we flip
	// the clock on regular game updates, which is not intended
	if (message.d.m) {
		// set frame time to compare against
		frameTime = performance.now();
		// reset centi-second clock interpolator to decrement correct player
		if (isPlayerTurn(message, ofenParts)) {
			frameId = requestAnimFrame(clockFrame(playerTimeRemaining, message.d.c.tc, plyTime, plyBar));
		} else {
			frameId = requestAnimFrame(clockFrame(opponentTimeRemaining, message.d.c.tc, oppTime, oppBar));
		}
	}
};

/**
 * Clock frame generator function
 * @param timeRemaining - last move message time
 * @param timeControl - time control, total seconds
 * @param timeElement - clock time element
 * @param barElement - clock progress bar element
 * @returns {(function(): void)|*} frame function
 */
const clockFrame = (timeRemaining, timeControl, timeElement, barElement) => () => {
	const elapsed = (performance.now() - frameTime) * 10;
	const remaining = ((timeRemaining * 100) - elapsed) / 100; // hacky
	timeElement.innerHTML = timeFormatter(Math.max(remaining, 0));
	barElement.style.width = barWidth(timeControl, remaining);

	frameId = requestAnimFrame(clockFrame(timeRemaining, timeControl, timeElement, barElement));
}

const padZero = (time, slice) => `0${time}`.slice(slice);

/**
 * Format time in MM:SS.CC
 * @param centiseconds - number of centi-seconds remaining
 * @returns {string} formatted time
 */
const timeFormatter = (centiseconds) => {
	const minutes = centiseconds / 6000 | 0;
	let minutesFmt;
	if (minutes > 9) {
		minutesFmt = padZero(centiseconds / 6000 | 0, 1);
	} else {
		minutesFmt = padZero(centiseconds / 6000 | 0, 0);
	}

	let seconds = (centiseconds / 100 | 0) % 60;
	if (seconds < 0) {
		seconds = 0;
	}
	const secondsFmt = padZero(seconds, -2);

	const centis = (centiseconds % 100);
	let centiFmt
	if (centis < 10) {
		centiFmt = padZero(centis, 0).slice(0, 1);
	} else {
		centiFmt = `${centis}`.slice(0, 1);
	}

	return `${minutesFmt}:${secondsFmt}.${centiFmt}`;
}

/**
 * Returns a CSS width percentage based on the percentage of
 * the clock time remaining for the given time control
 * @param timeControl - time control total centi-seconds
 * @param time - centi-seconds remaining
 * @returns {`${number}%`}
 */
const barWidth = (timeControl, time) => {
	return `${Math.min((time / timeControl) * 100, 100)}%`;
}

/**
 * Return a map of all legal moves
 * @param moves - raw moves object
 * @returns {Map<string, string[]>}
 */
const allMoves = (moves) => {
	let allMoves = new Map();
	if (!!moves) {
		Object.entries(moves).forEach(([s1, dests]) => {
			allMoves.set(s1, dests);
		});
	}
	return allMoves;
};

/**
 * Return the most recent game move for last move highlighting
 * @param moves - ordered list of all moves
 * @returns {[string, string]|*[]}
 */
const getLastMove = (moves) => {
	if (moves && moves.length > 0) {
		const move = moves[moves.length - 1];
		return [
			move.substring(0, 2),
			move.substring(2, 4)
		];
	}
	return [];
};

/**
 * Disable board if disconnected
 */
const disableBoard = () => {
	og.set({
		movable: {
			free: false,
			dests: new Map(),
		}
	});
};

/**
 * Perform move from origin to destination square and prompt for promotion
 * @param orig - origin square
 * @param dest - destination square
 */
const doMove = (orig, dest) => {
	if (og.state.pieces.get(dest) && og.state.pieces.get(dest).role === "pawn") {
		let destPiece = og.state.pieces.get(dest);
		if ((destPiece.color === "white" && dest[1] === "4") || (destPiece.color === "black" && dest[1] === "1")) {
			document.getElementById("promo-shade").classList.remove('hidden');

			let promoBar = document.getElementById("promo-select");
			promoBar.classList.remove('hidden');

			// set file for promo bar
			promoBar.classList.add(`f${dest[0]}`);

			// set piece selector colors and event handlers
			let promoButtons = promoBar.getElementsByTagName("piece");
			for (let i = 0; i < promoButtons.length; i++) {
				promoButtons[i].classList.add(destPiece.color);

				if (promoButtons[i].classList.contains("queen")) {
					promoButtons[i].addEventListener("click", () => doMovePromo(orig, dest, 'q'));
				} else if (promoButtons[i].classList.contains("rook")) {
					promoButtons[i].addEventListener("click", () => doMovePromo(orig, dest, 'r'));
				} else if ((promoButtons[i].classList.contains("bishop"))) {
					promoButtons[i].addEventListener("click", () => doMovePromo(orig, dest, 'b'));
				} else if (promoButtons[i].classList.contains("knight")) {
					promoButtons[i].addEventListener("click", () => doMovePromo(orig, dest, 'n'));
				}
			}

			// return early and wait for doMovePromo to run
			return
		}
	}

	sendGameMove(orig + dest, move);
	move++;
};

/**
 * Perform move from origin to destination square with selected promotion
 * @param orig - origin square
 * @param dest - destination square
 * @param promo - code of piece to promote to
 */
const doMovePromo = (orig, dest, promo) => {
	sendGameMove(orig + dest + promo, move);
	move++;

	// hide promo bar and shade after promotion
	document.getElementById("promo-shade").classList.add('hidden');

	let promoBar = document.getElementById("promo-select");

	// hide promo bar
	promoBar.classList.add('hidden');

	// unset file for promo bar
	promoBar.classList.remove(`f${dest[0]}`);

	// unset promo piece color
	let promoButtons = promoBar.getElementsByTagName("piece");
	for (let i = 0; i < promoButtons.length; i++) {
		promoButtons[i].classList.remove('white');
		promoButtons[i].classList.remove('black');
	}
}

/**
 * Play sounds for incoming moves based on the SAN for the move
 * and whether the move is a capture or results in check
 * @param message - move message
 */
const playMoveSound = (message) => {
	if (message.d.k) {
		window.checkSound.play();
		return;
	}

	if (message.d.s.includes("x")) {
		window.capSound.play();
	} else {
		window.moveSound.play();
	}
};

/**
 * Get cookie by name
 * @param cname
 * @returns {string}
 */
const getCookie = (cname) => {
	let name = cname + "=";
	let ca = document.cookie.split(';');
	for (let i = 0; i < ca.length; i++) {
		let c = ca[i];
		while (c.charAt(0) === ' ') {
			c = c.substring(1);
		}
		if (c.indexOf(name) === 0) {
			return c.substring(name.length, c.length);
		}
	}
	return "";
};

// Set handlers for game messages
window.handlers.set(moveTag, handleMove);
window.handlers.set(gameOverTag, handleGameOver);
