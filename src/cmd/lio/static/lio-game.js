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

// Move-list / review-navigation state. The server sends the full per-ply history
// (UOI + SAN + OFEN) on every board message, so the client can render the board
// at any past ply without an octad rules engine of its own. viewPly is the ply
// currently shown on the board; currentPly is the authoritative live tip. While
// followingLive is true the board tracks new moves as they arrive; while
// reviewing an earlier ply it stays put and only the move list grows.
// lastLiveMessage caches the most recent live board-state so returning to the
// tip can restore full interactivity (dests, turn, orientation).
let history = { uois: [], sans: [], ofens: [] };
let currentPly = 0;
let viewPly = 0;
let followingLive = true;
let lastLiveMessage = null;
// gameOver tracks whether the current game has ended, so returning to the live
// tip after review never re-enables dragging on a finished game (a resignation
// or timeout can leave a position that still has legal moves).
let gameOver = false;
// currentGameID tracks which game the board state we've applied belongs to
// (MovePayload.i). Game-boundary transitions (rematch reset, deploy reveal) are
// announced by single-shot broadcasts; if one is missed, the id changing on any
// later snapshot identifies the new game so it can be treated as a game start
// instead of being dropped by the gs/ply staleness guards (which break across
// game boundaries). null until the first id-carrying state arrives.
let currentGameID = null;

const moveTag = "m";
const gameOverTag = "g";
const rematchUpdateTag = "ru";
const drawOfferTag = "do";
const deployTag = "d";

// Blind deploy phase state. While deployMode is true the board is in
// "arrange your home rank" mode (drag/tap to swap your four pieces) rather than
// normal play; deployConfirmed locks the arrangement once submitted.
let deployMode = false;
let deploySpectating = false;  // watching a blind deploy phase (both ranks hidden)
let deployConfirmed = false;
let deployTimer = null;     // countdown setTimeout handle
let deployDeadline = 0;     // epoch ms when the server deploy window closes
let deployArrangement = null; // Map<square, piece> tracking our home-rank layout
let deployIsWhite = false;    // our side this game, from the deploy message ids
let deployLockWhite = false;  // white has committed its arrangement
let deployLockBlack = false;  // black has committed its arrangement

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
			if (deployMode) {
				onDeploySwap(orig, dest, capturedPiece);
			} else {
				doMove(orig, dest);
			}
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
// live countdown state, shared so the rematch click and the 'ru' update can
// relabel/retime the running countdown without losing the remaining seconds
let countdownRemaining = 0;
let countdownRender = null;
// human rematch-window state, reset on each new game over (see showResult)
let rematchRequested = false; // this client has clicked Rematch
let opponentLeft = false;     // the opponent disconnected during the window

// ---- in-game rail controls (resign / draw, swapped for rematch once over) ----
// The rail's control set (view/room.templ #game-controls) shows Resign/Draw
// during play and a Rematch button once the game is over (via the .controls-over
// class), so a player reviewing a finished game can rematch without the result
// overlay. The rail Rematch button shares its enable/disable/pending state with
// the overlay's #result-rematch (both listed in rematchButtons) and its
// data-rematch-url bot fallback, so the two stay in lockstep.
const gameControlsEl = document.getElementById('game-controls');
const resignBtn = document.getElementById('btn-resign');
const drawBtn = document.getElementById('btn-draw');
const railRematchBtn = document.getElementById('btn-rematch');

// two-step resign confirm state, and draw-offer state mirrored from the server's
// 'do' broadcasts (reset on each new game / whenever a move supersedes the offer)
let resignArmed = false;       // resign button is showing its confirm prompt
let resignArmTimer = null;     // auto-disarm timeout for the confirm prompt
let drawOfferedByMe = false;   // we have a standing draw offer out
let drawOfferedByOpp = false;  // the opponent has offered us a draw to accept

// the rematch buttons (overlay + rail) driven together as a set; capture each
// one's idle label once so pending/disabled states can restore it
const rematchButtons = [rematchBtn, railRematchBtn].filter(Boolean);
rematchButtons.forEach((b) => { b.dataset.defaultLabel = b.innerHTML; });

const setRematchButtonsDefault = () => rematchButtons.forEach((b) => {
	b.disabled = false;
	b.innerHTML = b.dataset.defaultLabel;
	b.classList.remove('wants-rematch');
});
const setRematchButtonsPending = () => rematchButtons.forEach((b) => {
	b.disabled = true;
	b.innerHTML = 'Waiting&hellip;';
});
// opponent left (human games): a rematch needs both players, so grey it out
const setRematchButtonsDisabled = () => rematchButtons.forEach((b) => {
	b.disabled = true;
	b.innerHTML = b.dataset.defaultLabel;
});
const setRematchButtonsWants = () => rematchButtons.forEach((b) => {
	if (!rematchRequested) { b.classList.add('wants-rematch'); }
});
const clearRematchButtonsWants = () => rematchButtons.forEach((b) => {
	b.classList.remove('wants-rematch');
});

/**
 * setControlsMode swaps the rail control set between live play (Resign / Draw)
 * and game-over (Rematch). Toggling .controls-over does the visual swap; the
 * play-control state is reset either way so a stale confirm/offer never lingers.
 * @param over - true once the game is over (show Rematch), false during play
 */
const setControlsMode = (over) => {
	if (gameControlsEl) {
		gameControlsEl.classList.toggle('controls-over', over);
	}
	resetResignButton();
	clearDrawOfferUI();
};

/**
 * resetResignButton returns the Resign button to its idle (un-armed) state and
 * cancels any pending auto-disarm.
 */
const resetResignButton = () => {
	resignArmed = false;
	if (resignArmTimer !== null) {
		clearTimeout(resignArmTimer);
		resignArmTimer = null;
	}
	if (resignBtn) {
		resignBtn.classList.remove('confirm');
		resignBtn.innerHTML = '⚑ Resign';
	}
};

/**
 * clearDrawOfferUI drops any draw-offer affordance and returns the Draw button
 * to its idle state.
 */
const clearDrawOfferUI = () => {
	drawOfferedByMe = false;
	drawOfferedByOpp = false;
	if (drawBtn) {
		drawBtn.classList.remove('wants-draw');
		drawBtn.disabled = false;
		drawBtn.innerHTML = '½ Draw';
	}
};

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
 * Stop and clear the result-overlay countdown.
 */
const stopCountdown = () => {
	if (countdownInterval !== null) {
		clearInterval(countdownInterval);
		countdownInterval = null;
	}
	countdownRemaining = 0;
	countdownRender = null;
	if (resultCountdownEl) {
		resultCountdownEl.innerHTML = '';
		resultCountdownEl.classList.remove('opponent-left');
	}
};

/**
 * Start a one-per-second countdown under the result actions. renderFn(remaining)
 * returns the HTML for the current state; it is re-read every tick (and via
 * refreshCountdownLabel) so a state change mid-window relabels in place. The
 * new-game / room-over broadcast clears it via hideResult / stopCountdown.
 * @param seconds - whole seconds to count down from
 * @param renderFn - (remaining) => html string
 */
const startCountdown = (seconds, renderFn) => {
	stopCountdown();
	if (!resultCountdownEl || !seconds || seconds <= 0) {
		return;
	}

	countdownRemaining = seconds;
	countdownRender = renderFn;
	const tick = () => {
		resultCountdownEl.innerHTML = countdownRender(countdownRemaining);
	};
	tick();

	countdownInterval = setInterval(() => {
		countdownRemaining -= 1;
		tick();
		if (countdownRemaining <= 0) {
			clearInterval(countdownInterval);
			countdownInterval = null;
		}
	}, 1000);
};

/**
 * Re-render the running countdown immediately with its current remaining time,
 * e.g. after rematchRequested / opponentLeft changed mid-window.
 */
const refreshCountdownLabel = () => {
	if (countdownInterval !== null && countdownRender && resultCountdownEl) {
		resultCountdownEl.innerHTML = countdownRender(countdownRemaining);
	}
};

/**
 * Countdown label for human games: the room closes when the window lapses
 * unless both players agree a rematch. Copy follows the rematch state.
 */
const rematchWindowLabel = (remaining) => {
	if (remaining <= 0) {
		return 'Closing&hellip;';
	}
	if (opponentLeft) {
		return `Opponent left &middot; ${remaining}s`;
	}
	if (rematchRequested) {
		return `Waiting for opponent &middot; ${remaining}s`;
	}
	return `Rematch &middot; ${remaining}s`;
};

// Post-rematch resync safety net. The start of the next game is announced by a
// single server broadcast — the blind-deploy 'd' message in deploy variants, or
// the gs=true board reset otherwise. If this client misses that one message
// (a momentary socket hiccup, a full send buffer, a throttled background tab at
// exactly the wrong instant) nothing else pulls it into the new game, so it
// sits on the "Waiting…" overlay until the 30s server-side deploy autofill — the
// "rematch gets stuck" symptom. While we are waiting for a rematch we clicked to
// start, we poll for authoritative state: the server answers an a:0 board query
// with the deploy-start message during the deploy phase (see handle_move.go) or
// the current game state otherwise, either of which resyncs us. The poll is
// cleared the moment the next game actually begins — enterDeployMode and the
// gs=true branch of handleMove both run hideResult — so the happy path (deploy
// arrives within ~1s) sends zero extra traffic. See
// arch/DEPLOY_REMATCH_RACES.md (race #1).
let rematchResyncTimer = null;
const rematchResyncIntervalMs = 2000;

const startRematchResync = () => {
	stopRematchResync();
	rematchResyncTimer = setInterval(() => {
		sendBoardUpdateRequest();
	}, rematchResyncIntervalMs);
};

const stopRematchResync = () => {
	if (rematchResyncTimer !== null) {
		clearInterval(rematchResyncTimer);
		rematchResyncTimer = null;
	}
};

/**
 * Hide the game-end result overlay and reset its rematch button and countdown.
 */
const hideResult = () => {
	if (resultOverlayEl) {
		resultOverlayEl.classList.remove('result-show');
		// clear any "dismissed for analysis" state so the next game's overlay is clean
		resultOverlayEl.classList.remove('result-dismissed');
	}
	const restoreBtn = document.getElementById('result-restore');
	if (restoreBtn) {
		restoreBtn.classList.add('hidden');
	}
	setRematchButtonsDefault();
	rematchRequested = false;
	opponentLeft = false;
	hideOpponentRematchRequest();
	// the overlay is gone: a new/live game is in play, so return the rail controls
	// to Resign / Draw (and reset any lingering confirm/offer state)
	setControlsMode(false);
	// the next game has started (or the overlay is being torn down): stop polling
	stopRematchResync();
	stopCountdown();
};

/**
 * showOpponentRematchRequest surfaces that the opponent asked for a rematch, so
 * the player knows a single click will start the next game. Highlights the
 * rematch button and shows a note.
 */
const showOpponentRematchRequest = () => {
	const note = document.getElementById('result-note');
	if (note) {
		note.textContent = 'Opponent wants a rematch';
		note.classList.remove('hidden');
	}
	// draw the eye to the action unless we've already committed to it
	setRematchButtonsWants();
};

const hideOpponentRematchRequest = () => {
	const note = document.getElementById('result-note');
	if (note) {
		note.classList.add('hidden');
		note.textContent = '';
	}
	clearRematchButtonsWants();
};

/**
 * Populate and show the game-end result overlay from a game-over message.
 * @param message - game over message
 */
const showResult = (message) => {
	if (!resultOverlayEl) {
		return;
	}

	// a fresh game-over: clear any human rematch-window state carried over from
	// a previous game in this room, and re-enable the rematch action
	rematchRequested = false;
	opponentLeft = false;
	hideOpponentRematchRequest();
	setRematchButtonsDefault();

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

	// a rematch is impossible once the whole room is over (abandon / match end):
	// hide the overlay's button and disable the rail's
	const roomOver = !!message.d.o;
	if (rematchBtn) {
		rematchBtn.style.display = roomOver ? 'none' : '';
	}
	if (railRematchBtn && roomOver) {
		railRematchBtn.disabled = true;
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

	// human games tick the manual-rematch window (rw) down to the room closing;
	// bot games are not time-boxed and carry no countdown (the finished room stays
	// open for review + manual rematch), so a missing rw just clears the countdown.
	if (message.d.rw) {
		startCountdown(message.d.rw, rematchWindowLabel);
	} else {
		stopCountdown();
	}

	resultOverlayEl.classList.add('result-show');
};

/**
 * requestRematch drives a rematch from either the result-overlay or the rail
 * Rematch button. Bot rematch reuses this finished room via the in-room
 * agreement flow: the server auto-agrees on the bot's behalf
 * (room/handle_04_game_over.go), so one click starts the next game in the SAME
 * room with the running match score preserved. Only when that room is genuinely
 * gone — the player reviewed the game past the bot analysis window and the socket
 * has since closed — do we fall back to spinning up a fresh room (a new 0-0
 * series). data-rematch-url is set for bot games only, so this fallback never
 * fires for human games.
 * @param btn - the clicked rematch button (carries the bot fallback url)
 */
const requestRematch = (btn) => {
	const socketDead = !window.ws || window.ws.readyState !== WebSocket.OPEN;
	const fallbackUrl = btn && btn.dataset.rematchUrl;
	if (socketDead && fallbackUrl) {
		window.location.href = fallbackUrl;
		return;
	}
	// in-room agreement flow (humans always; bots while the room is alive)
	send(buildCommand("r", {rm: true}));
	// leave the overlay up until the rematch actually starts; mark the request
	// sent and reflect it on both rematch buttons + the still-running countdown
	// (human games relabel to "Waiting for opponent").
	setRematchButtonsPending();
	rematchRequested = true;
	hideOpponentRematchRequest();
	// guard against missing the single next-game / deploy-start broadcast: poll
	// for authoritative state until the new game begins (see startRematchResync).
	startRematchResync();
	refreshCountdownLabel();
};

// wire both rematch buttons once; the new-game broadcast clears the overlay
rematchButtons.forEach((b) => b.addEventListener('click', () => requestRematch(b)));
if (homeBtn) {
	homeBtn.addEventListener('click', () => {
		window.location.href = "/";
	});
}

// Resign is a two-step confirm so a mis-click can't throw the game: the first
// click arms the button ("Confirm?"), a second within the window sends it, and
// it auto-disarms after a few seconds. The server (RequestResign) only accepts
// it from a seated player during an ongoing game.
if (resignBtn) {
	resignBtn.addEventListener('click', () => {
		if (gameOver) {
			return;
		}
		if (!resignArmed) {
			resignArmed = true;
			resignBtn.classList.add('confirm');
			resignBtn.innerHTML = 'Confirm?';
			resignArmTimer = setTimeout(resetResignButton, 4000);
			return;
		}
		resetResignButton();
		send(buildCommand("r", {rs: true}));
	});
}

// Draw offers/accepts a draw. If the opponent has a standing offer this click
// accepts it (the game ends by agreement); otherwise it sends our offer and
// shows a pending state until the opponent answers, the bot's engine decides, or
// a move supersedes it. The button doubles as "Accept draw" while an opponent
// offer stands (see handleDrawOffer).
if (drawBtn) {
	drawBtn.addEventListener('click', () => {
		if (gameOver || drawOfferedByMe) {
			return;
		}
		send(buildCommand("r", {dr: true}));
		if (drawOfferedByOpp) {
			// accepting the opponent's standing offer; the game will end shortly
			drawBtn.disabled = true;
			drawBtn.classList.remove('wants-draw');
		} else {
			// we offered: reflect pending until it is answered / superseded
			drawOfferedByMe = true;
			drawBtn.disabled = true;
			drawBtn.innerHTML = 'Offered&hellip;';
		}
	});
}

/**
 * handleDrawOffer reflects server draw-offer state ('do'): a standing offer
 * (by = offering uid) shows a pending state for the offerer and an "accept draw"
 * affordance for the opponent; a decline (dc) briefly notes it and restores the
 * button. Offers are also cleared locally when a move arrives (see handleMove).
 * @param message - draw offer message
 */
const handleDrawOffer = (message) => {
	if (gameOver) {
		return;
	}
	const d = message.d || {};
	if (d.dc) {
		// a standing offer was declined (bot) or withdrawn; note it, then reset
		const wasPending = drawOfferedByMe || drawOfferedByOpp;
		clearDrawOfferUI();
		if (wasPending && drawBtn) {
			drawBtn.disabled = true;
			drawBtn.innerHTML = 'Declined';
			setTimeout(() => {
				if (!gameOver && drawBtn) {
					drawBtn.disabled = false;
					drawBtn.innerHTML = '½ Draw';
				}
			}, 1500);
		}
		return;
	}
	if (!d.by) {
		return;
	}
	if (d.by === getCookie('uid')) {
		// our own offer, echoed by the server: reflect the pending state
		drawOfferedByMe = true;
		if (drawBtn) {
			drawBtn.disabled = true;
			drawBtn.innerHTML = 'Offered&hellip;';
		}
	} else {
		// the opponent offered a draw: turn Draw into an accept affordance
		drawOfferedByOpp = true;
		if (drawBtn) {
			drawBtn.disabled = false;
			drawBtn.classList.add('wants-draw');
			drawBtn.innerHTML = '½ Accept draw';
		}
	}
};

// "Analyze board" dismisses the result card (without tearing down its state, so
// the rematch window / countdown keep running and Rematch stays reachable) and
// exposes a small floating button to bring the card back. This lets a player
// step through the finished game with the board unobstructed.
const analyzeBtn = document.getElementById('result-analyze');
const restoreResultBtn = document.getElementById('result-restore');
if (analyzeBtn) {
	analyzeBtn.addEventListener('click', () => {
		if (resultOverlayEl) {
			resultOverlayEl.classList.add('result-dismissed');
		}
		if (restoreResultBtn) {
			restoreResultBtn.classList.remove('hidden');
		}
	});
}
if (restoreResultBtn) {
	restoreResultBtn.addEventListener('click', () => {
		if (resultOverlayEl) {
			resultOverlayEl.classList.remove('result-dismissed');
		}
		restoreResultBtn.classList.add('hidden');
	});
}

/**
 * Handle incoming move messages, update board state, update UI and clocks
 * @param message - move message
 */
const handleMove = (message) => {
	// game identity: a snapshot carrying a different game id than the one we're
	// tracking is our first sight of a new game — we missed its single-shot
	// start broadcast (gs=true reveal or 'd' deploy start) — so it must be
	// handled as a game start rather than dropped by the staleness guards
	// below, which all break across a game boundary. The very first id after a
	// page load is adopted silently (it describes the game already in
	// progress, not a boundary we crossed).
	const gid = message.d.i;
	const newGame = !!message.d.gs || (!!gid && !!currentGameID && gid !== currentGameID);
	if (gid) {
		currentGameID = gid;
	}

	// while arranging or spectating the blind deploy phase, ignore stale pre-deploy
	// board states (r.game is still the previous position server-side); only a
	// new game's board state (the reveal, or any later snapshot of it if the
	// reveal was missed) ends the phase and renders pieces
	if ((deployMode || deploySpectating) && !newGame) {
		return;
	}

	// while waiting for a clicked rematch to start, ignore stale board states
	// for the finished game. The resync poll (startRematchResync) re-requests
	// state on an interval, and the finished game's position can still carry
	// legal moves (a resignation or timeout ends the game with a playable
	// board), which would otherwise re-enable dragging behind the result
	// overlay. Only the next game — recognized by gs or its id — pulls us
	// forward (a 'd' deploy message is routed elsewhere).
	if (rematchRequested && !newGame) {
		return;
	}

	const ofenParts = message.d.o.split(' ');
	const serverPly = messagePly(message);

	// ignore a stale board snapshot that would regress the board to an older
	// position (e.g. a late board-state response landing after newer state). A
	// game start legitimately resets the ply, so always honor it.
	if (!newGame && serverPly < lastPly) {
		return;
	}

	// a played move supersedes any standing draw offer (the server withdraws it
	// on the move too); drop the affordance so a stale "accept draw" / "offered…"
	// can't linger past the position it referred to
	if (!newGame && serverPly > lastPly) {
		clearDrawOfferUI();
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
	currentPly = serverPly;

	// capture the authoritative per-ply history for the move list + navigation
	history = {
		uois: message.d.m || [],
		sans: message.d.sm || [],
		ofens: message.d.om || [],
	};

	if (!message.d.m) {
		move = 1;
		document.getElementById("info").innerHTML = "";
	}

	// cache the player's color for the result overlay, and clear any prior
	// result when a new game (e.g. a rematch) starts
	playerWhite = isPlayerWhite(message);
	if (newGame) {
		gameOver = false;
		// a new game always returns us to the live board
		followingLive = true;
		hideResult();
		// the first game-state after the deploy phase is the reveal: drop the
		// blind overlay and restore normal play before rendering the position
		if (deployMode || deploySpectating) {
			exitDeployMode();
		}
		// a new game invalidates any move left unconfirmed from the prior one
		clearPending();
	} else if (!followingLive && message.d.m
		&& isPlayerParticipant(message) && isPlayerTurn(message, ofenParts)) {
		// snap a reviewing player back to the live board the moment it becomes
		// their turn to move, so they're never left unable to play
		followingLive = true;
	}

	// always keep the latest authoritative live state so a return-to-live can
	// restore full interactivity even while we're currently reviewing an earlier ply
	lastLiveMessage = message;

	// only render the board (and play move sounds / premove) while following the
	// live tip; while reviewing an earlier ply the board stays put and just the
	// move list and live clocks update
	if (followingLive) {
		viewPly = currentPly;
		playSounds(message, ofenParts, newGame);
		renderLivePosition(message, ofenParts);
	}

	// update UI styles and clock tickers (live state, regardless of view)
	updateUI(message, ofenParts);

	// show/hide the engine thinking indicator based on whose turn it is
	updateThinking(message, ofenParts);

	// perform pre-move if set
	if (followingLive) {
		og.playPremove();
	}

	// refresh the move list so it always reflects the live tip
	renderMoveList();
};

/**
 * renderLivePosition applies the authoritative live board-state to octadground:
 * position, orientation, last-move highlight, turn, check, and legal-move
 * destinations. Interactivity is suppressed once the game is over so returning
 * to the live tip after review never re-enables dragging on a finished board.
 * @param message - move message
 * @param ofenParts - OFEN parts array
 */
const renderLivePosition = (message, ofenParts) => {
	og.set({
		orientation: message.d.w === getCookie('uid') ? 'white' : 'black',
		ofen: ofenParts[0],
		lastMove: getLastMove(message.d.m),
		turnColor: whiteToMove(ofenParts) ? "white" : "black",
		check: message.d.k,
		movable: {
			free: false,
			dests: gameOver ? new Map() : allMoves(message.d.v),
			color: message.d.w === getCookie('uid') ? 'white' : 'black',
		}
	});
};

/**
 * isPlayerParticipant returns true if the viewer is one of the two seated
 * players (not a spectator), by matching their uid against the message's player
 * ids. Used to decide whether a "your turn" auto-snap applies.
 * @param message - move message carrying white/black player ids
 */
const isPlayerParticipant = (message) => {
	const uid = getCookie('uid');
	return message.d.w === uid || message.d.b === uid;
};

/**
 * isAnalyzing reports whether the player is actively reviewing the finished game
 * — either they dismissed the result card ("Analyze board") or they've stepped
 * the board back to an earlier ply. Used to keep an analyzing player on the page
 * when a finished bot room is torn down, instead of bouncing them home.
 */
const isAnalyzing = () => {
	const dismissed = !!resultOverlayEl && resultOverlayEl.classList.contains('result-dismissed');
	return dismissed || !followingLive;
};

/**
 * Handles incoming game over messages
 * @param message - game over message
 */
const handleGameOver = (message) => {
	// A repeat of the game-over we're already showing: while a player waits out
	// the rematch window, the resync poll's a:0 queries are answered with the
	// game-over payload again (handle_move.go's StateGameOver branch). A repeat
	// must not reset the rematch UI — showResult would silently un-click a sent
	// rematch request (rematchRequested = false) and disarm the stale-board
	// guard — nor replay the notification sound every poll tick. Just retime
	// the countdown to the server's authoritative remaining window. A room-over
	// notice (o) is never a repeat; it always runs the full path below.
	if (gameOver && message.d.o !== true
		&& resultOverlayEl && resultOverlayEl.classList.contains('result-show')) {
		if (message.d.rw) {
			startCountdown(message.d.rw, rematchWindowLabel);
		}
		updateScore(message);
		return;
	}

	cancelAnimationFrame(frameId);
	document.getElementById("info").innerHTML = message.d.s;
	window.notification.play();

	// the game has ended: keep the board non-interactive even if the player
	// reviews and returns to the live tip (see renderLivePosition)
	gameOver = true;

	// swap the rail's Resign / Draw controls for the Rematch button so a player
	// reviewing the finished game can rematch without the result overlay
	setControlsMode(true);

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

	// if room over, redirect home after a moment
	if (message.d.o === true) {
		// no next game is coming; stop any post-rematch resync poll so it can't
		// outlive the room while we wait to redirect
		stopRematchResync();

		// A finished bot room is torn down after its analysis window. If the player
		// is reviewing the game, keep them on the page — analysis is client-side and
		// the now-closed room means Rematch falls back to spinning up a fresh room
		// (the socket is closed just below) — instead of bouncing them home. Stop
		// auto-reconnect so the client doesn't fight the now-gone room.
		if (opponentIsBot && isAnalyzing()) {
			if (typeof window.lioStopReconnect === 'function') {
				window.lioStopReconnect();
			}
			return;
		}

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
 * @param newGame - the message is a game start (gs flag, or a game-id change
 *                  recognized in handleMove after a missed start broadcast)
 */
const playSounds = (message, ofenParts, newGame) => {
	if (newGame) {
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

// previous match score (raw white/black), so updateScore can flash only the side
// whose score actually changed at game end. null until the first score message.
let prevScoreW = null, prevScoreB = null;

/**
 * Flash a score element's end-of-game delta: green for a win (+1), grey for a
 * draw (+½). No-op unless the score went up.
 * @param el - the .clockScore span
 * @param delta - change since the last score
 */
const flashScore = (el, delta) => {
	if (!el || delta <= 0) {
		return;
	}
	el.classList.remove('score-win', 'score-draw');
	void el.offsetWidth; // reflow so re-adding the class restarts the animation
	el.classList.add(delta >= 0.75 ? 'score-win' : 'score-draw');
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

	const w = message.d.sc.w || 0;
	const b = message.d.sc.b || 0;

	// resolve which clock element is white vs black so the flash lands correctly
	const whiteScore = isPlayerWhite(message) ? plyScore : oppScore;
	const blackScore = isPlayerWhite(message) ? oppScore : plyScore;

	whiteScore.innerHTML = w;
	blackScore.innerHTML = b;

	// match score only changes at game end, so a positive delta is the trigger
	if (prevScoreW !== null) {
		flashScore(whiteScore, w - prevScoreW);
		flashScore(blackScore, b - prevScoreB);
	}
	prevScoreW = w;
	prevScoreB = b;
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
 * playNavSound plays the check/capture/move sound for a SAN string while
 * stepping through the game (nav buttons, move-list clicks, arrow keys). The
 * server-sent SAN carries '+'/'#' for check/mate and 'x' for captures. A falsy
 * san (the start position, which has no preceding move) plays the plain move
 * sound so the ⏮ / go-to-start control isn't silent.
 * @param san - SAN of the move leading into the viewed ply, or falsy for start
 */
const playNavSound = (san) => {
	if (san && (san.includes("#") || san.includes("+"))) {
		window.checkSound.play();
	} else if (san && san.includes("x")) {
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

/**
 * Handle a rematch-window update: the server retimed the human rematch window
 * mid-window — e.g. the opponent disconnected and it was shortened, or they
 * returned and it was restored. Retime the countdown in place; once the
 * opponent has left a rematch is impossible, so disable the action.
 * @param message - rematch update message
 */
const handleRematchUpdate = (message) => {
	// only meaningful while the result overlay is showing a live window
	if (!resultOverlayEl || !resultOverlayEl.classList.contains('result-show')) {
		return;
	}

	// a rematch-request signal ({rq: requester id}): surface it to the opponent
	// only (our own click already shows "Waiting…"), and don't retime the window
	if (message.d.rq) {
		if (message.d.rq !== getCookie('uid')) {
			showOpponentRematchRequest();
		}
		return;
	}

	opponentLeft = !!message.d.ol;
	if (opponentLeft) {
		// a rematch needs both players; once the opponent leaves it can't happen —
		// grey out both the overlay and rail rematch buttons
		setRematchButtonsDisabled();
	} else if (rematchRequested) {
		// opponent returned within the grace and we had already asked for a
		// rematch: the server still holds our recorded agreement, so restore the
		// pending "waiting" state rather than silently un-clicking (which would
		// desync the UI from the server and disarm the stale-board guard)
		setRematchButtonsPending();
	} else {
		// opponent returned within the grace: offer the rematch afresh
		setRematchButtonsDefault();
	}

	// startCountdown resets the countdown element; (re)apply the amber
	// opponent-left highlight afterwards so it reflects the current state
	startCountdown(message.d.s, rematchWindowLabel);
	if (resultCountdownEl) {
		resultCountdownEl.classList.toggle('opponent-left', opponentLeft);
	}
};

/**
 * homeRankSquares returns the player's four home-rank squares in display
 * left-to-right order (the order the server expects the deploy string in). For
 * white that is a1..d1; for black the board is flipped so it is d4..a4.
 */
const homeRankSquares = (white) => white ? ['a1', 'b1', 'c1', 'd1'] : ['d4', 'c4', 'b4', 'a4'];

/**
 * deployMovable builds the octadground movable config that restricts moves to
 * swaps within the player's own home rank during the deploy phase.
 */
const deployMovable = (white) => {
	const sqs = white ? ['a1', 'b1', 'c1', 'd1'] : ['a4', 'b4', 'c4', 'd4'];
	const dests = new Map();
	for (const s of sqs) {
		dests.set(s, sqs.filter(x => x !== s));
	}
	return { free: false, color: white ? 'white' : 'black', dests: dests };
};

/**
 * handleDeploy processes blind deploy-phase messages: the phase-start / reconnect
 * message ({a, s, w, b, ...}) enters (or restores) deploy mode, and a lock update
 * ({lk: color}) marks a side as committed without re-entering the phase.
 */
const handleDeploy = (message) => {
	const d = message.d || {};

	// a lock update ({lk}) reports a side committed; update the indicator only
	if (d.lk) {
		updateDeployLock(d.lk);
		return;
	}

	const uid = getCookie('uid');
	const seconds = d.s ? d.s : 30;
	// derive our side from the message's player ids rather than the DOM
	// orientation class, which is stale after a rematch swaps colors. A spectator
	// matches neither id and watches the blind phase (both ranks hidden).
	if (uid !== d.w && uid !== d.b) {
		enterDeploySpectatorMode(d);
		return;
	}
	deployIsWhite = (uid === d.w);
	enterDeployMode(seconds, d);
};

const enterDeployMode = (seconds, payload) => {
	const d = payload || {};
	if (deployMode) {
		// already arranging (e.g. a reconnect's deploy-state response): refresh
		// lock indicators and reconcile confirmation. If we believe we confirmed
		// but the server has no arrangement for us (cf absent), the submission
		// was lost in transit — clear the latch and resend the current
		// arrangement, otherwise we'd sit on "waiting" until the server's
		// deploy-timeout autofill while the opponent waits out the full window.
		if (d.lw) { updateDeployLock('white'); }
		if (d.lb) { updateDeployLock('black'); }
		if (deployConfirmed && !d.cf) {
			deployConfirmed = false;
			confirmDeploy();
		}
		return;
	}
	deployMode = true;
	deployConfirmed = false;
	deployLockWhite = false;
	deployLockBlack = false;

	// a deploy phase begins a new game, so clear any lingering game-over /
	// rematch overlay from the previous game
	hideResult();

	const white = deployIsWhite;
	const myColor = white ? 'white' : 'black';
	// show only the player's own pieces in standard order; the opponent's rank is
	// covered by the "?" overlay and the middle ranks are empty
	const ofen = white ? '4/4/4/NKPP' : 'ppkn/4/4/4';
	og.set({
		ofen: ofen,
		orientation: myColor,
		turnColor: myColor,
		lastMove: undefined,
		check: false,
		// disable octadground's auto-castle: dragging the king two squares onto a
		// same-color pawn/knight would otherwise trigger a castling relocation
		// (king one step over, partner to the king's square) and duplicate a piece.
		// Deploy swaps are reconstructed by onDeploySwap, so plain moves are wanted.
		autoCastle: false,
		draggable: { enabled: true },
		selectable: { enabled: true },
		movable: deployMovable(white),
	});

	// snapshot our starting home-rank layout so swaps can be reconstructed
	// (octadground overwrites the destination piece on a same-color move)
	deployArrangement = new Map();
	for (const sq of homeRankSquares(white)) {
		deployArrangement.set(sq, og.state.pieces.get(sq));
	}

	document.getElementById('deploy-questions').classList.add('deploy-show');
	document.getElementById('deploy-overlay').classList.add('deploy-show');
	const btn = document.getElementById('deploy-confirm');
	btn.classList.remove('hidden');
	btn.disabled = false;
	btn.onclick = confirmDeploy;
	document.getElementById('deploy-waiting').classList.add('hidden');

	startDeployCountdown(seconds);

	// restore a prior arrangement across a refresh: the server replays our own
	// confirmed order (d.o); otherwise fall back to an unconfirmed local draft.
	const draft = loadDeployDraft();
	const restoreOrder = d.o || (draft && draft.o);
	if (restoreOrder) {
		applyDeployOrder(restoreOrder);
	}
	// re-lock if we had already confirmed (server-known, or a confirmed draft)
	if (d.cf || (draft && draft.cf)) {
		confirmDeploy();
	}
	// reflect any side already locked in (reconnect)
	if (d.lw) { updateDeployLock('white'); }
	if (d.lb) { updateDeployLock('black'); }
};

/**
 * enterDeploySpectatorMode shows the blind deploy phase to a spectator: an empty
 * board with both home ranks hidden behind "?" cells and a passive, control-free
 * card. It also reflects each side's locked-in status as it arrives.
 */
const enterDeploySpectatorMode = (payload) => {
	const d = payload || {};
	if (deploySpectating) {
		if (d.lw) { updateDeployLock('white'); }
		if (d.lb) { updateDeployLock('black'); }
		return;
	}
	deploySpectating = true;
	deployLockWhite = false;
	deployLockBlack = false;
	hideResult();

	// blank the board; both home ranks sit behind the "?" overlay
	og.set({
		ofen: '4/4/4/4',
		lastMove: undefined,
		check: false,
		draggable: { enabled: false },
		selectable: { enabled: false },
		movable: { free: false, color: undefined, dests: new Map() },
	});

	document.getElementById('deploy-questions').classList.add('deploy-show');
	document.getElementById('deploy-questions-btm').classList.add('deploy-show');

	// passive spectator card: no controls, just a status line
	document.getElementById('deploy-overlay').classList.add('deploy-show');
	document.querySelector('.deploy-headline').textContent = 'Blind deploy';
	document.querySelector('.deploy-hint').textContent = 'Both players are secretly arranging their pieces.';
	document.getElementById('deploy-countdown').classList.add('hidden');
	document.getElementById('deploy-confirm').classList.add('hidden');
	document.getElementById('deploy-waiting').classList.add('hidden');

	if (d.lw) { updateDeployLock('white'); }
	if (d.lb) { updateDeployLock('black'); }
	renderDeployLock();
};

/**
 * applyDeployOrder rearranges the player's home rank to a saved 4-char order
 * (k/n/p in display left-to-right order) and rebuilds the tracked arrangement.
 */
const applyDeployOrder = (order) => {
	if (!order || order.length !== 4) {
		return;
	}
	const color = deployIsWhite ? 'white' : 'black';
	const letterToRole = { k: 'king', n: 'knight', p: 'pawn' };
	const sqs = homeRankSquares(deployIsWhite);
	const pieces = new Map();
	const next = new Map();
	for (let i = 0; i < 4; i++) {
		const role = letterToRole[order[i]];
		if (!role) {
			return; // malformed; keep the standard order already on the board
		}
		const piece = { role: role, color: color };
		pieces.set(sqs[i], piece);
		next.set(sqs[i], piece);
	}
	deployArrangement = next;
	og.setPieces(pieces);
};

/**
 * onDeploySwap completes a swap when a piece is dragged/tapped onto another of
 * the player's home-rank pieces. octadground has already moved orig->dest and
 * overwritten the destination piece, so we reconstruct the swap from our tracked
 * arrangement: the displaced piece slides back to the origin.
 */
const onDeploySwap = (orig, dest) => {
	if (!deployArrangement) {
		return;
	}
	const moved = deployArrangement.get(orig);
	const displaced = deployArrangement.get(dest);
	if (!moved) {
		return;
	}

	// update our model first
	deployArrangement.set(dest, moved);
	deployArrangement.set(orig, displaced);
	const white = deployIsWhite;

	// a piece just changed squares — same feedback as a real move
	playSwapSound();
	// remember the in-progress arrangement so a refresh restores it (see #3)
	saveDeployDraft();

	// defer the re-render so octadground finishes its own drag/move handling
	// before we re-assert both squares and re-apply the deploy move restriction
	setTimeout(() => {
		// seed octadground's pre-anim snapshot with the displaced piece still on
		// dest (the dragged piece already sits there), so the follow-up setPieces
		// animates the displaced piece sliding dest -> orig instead of popping in.
		if (displaced) {
			og.state.pieces.set(dest, displaced);
		} else {
			og.state.pieces.delete(dest);
		}
		og.state.pieces.delete(orig);
		og.setPieces(new Map([[dest, moved], [orig, displaced || null]]));
		// octadground flips turnColor after a user move; restore ours so the next
		// swap is allowed, and re-assert the home-rank move restriction
		og.set({ turnColor: white ? 'white' : 'black', movable: deployMovable(white) });
	}, 0);
};

/**
 * playSwapSound plays the standard move sound for a deploy rearrangement,
 * mirroring the feedback of a real move.
 */
const playSwapSound = () => {
	if (window.moveSound) {
		window.moveSound.play();
	}
};

/**
 * readDeployOrder reads the player's home-rank arrangement into the 4-char
 * order string (k/n/p) the server expects, in the player's display order.
 */
const readDeployOrder = () => {
	const roleToLetter = { king: 'k', knight: 'n', pawn: 'p' };
	let order = '';
	for (const sq of homeRankSquares(deployIsWhite)) {
		const piece = og.state.pieces.get(sq);
		order += piece ? (roleToLetter[piece.role] || '') : '';
	}
	return order;
};

const confirmDeploy = () => {
	if (deployConfirmed || !deployMode) {
		return;
	}
	deployConfirmed = true;
	// persist the confirmed state so a refresh re-enters locked, not unconfirmed
	saveDeployDraft(true);
	send(buildCommand(deployTag, { o: readDeployOrder() }));
	// lock the board and switch the controls to the waiting state
	og.set({ draggable: { enabled: false }, selectable: { enabled: false }, movable: { free: false, color: undefined, dests: new Map() } });
	document.getElementById('deploy-confirm').classList.add('hidden');
	document.getElementById('deploy-waiting').classList.remove('hidden');
	renderDeployLock();
};

/**
 * readDeployOrderFromModel reads the 4-char order (k/n/p) from our tracked
 * arrangement rather than the live board — safe to call mid-swap, before the
 * deferred board re-render has run.
 */
const readDeployOrderFromModel = () => {
	const roleToLetter = { king: 'k', knight: 'n', pawn: 'p' };
	let order = '';
	for (const sq of homeRankSquares(deployIsWhite)) {
		const piece = deployArrangement ? deployArrangement.get(sq) : null;
		order += piece ? (roleToLetter[piece.role] || '') : '';
	}
	return order;
};

// deployDraftKey namespaces the saved arrangement to this room (the URL path).
const deployDraftKey = () => 'deploy:' + window.location.pathname;

/**
 * saveDeployDraft mirrors the in-progress (or confirmed) arrangement to
 * sessionStorage so a refresh mid-deploy restores it — the server never sees an
 * unconfirmed arrangement, so the client is the only place it can survive.
 */
const saveDeployDraft = (confirmed) => {
	try {
		sessionStorage.setItem(deployDraftKey(), JSON.stringify({
			o: readDeployOrderFromModel(),
			w: deployIsWhite,
			cf: !!confirmed,
		}));
	} catch (e) { /* storage unavailable; drafts just won't persist */ }
};

const loadDeployDraft = () => {
	try {
		const raw = sessionStorage.getItem(deployDraftKey());
		if (!raw) { return null; }
		const draft = JSON.parse(raw);
		// only honor a draft that matches our current side (a rematch can swap it)
		if (draft && draft.w === deployIsWhite && typeof draft.o === 'string' && draft.o.length === 4) {
			return draft;
		}
	} catch (e) { /* ignore malformed drafts */ }
	return null;
};

const clearDeployDraft = () => {
	try { sessionStorage.removeItem(deployDraftKey()); } catch (e) { /* noop */ }
};

/**
 * updateDeployLock records that a color committed its arrangement and refreshes
 * the "locked in" indicator (opponent-only for a player, both sides for a
 * spectator).
 */
const updateDeployLock = (color) => {
	if (color === 'white') { deployLockWhite = true; }
	else if (color === 'black') { deployLockBlack = true; }
	renderDeployLock();
};

const renderDeployLock = () => {
	const el = document.getElementById('deploy-opponent-status');
	if (!el) { return; }
	if (deploySpectating) {
		el.textContent = 'White: ' + (deployLockWhite ? 'ready ✓' : 'arranging…')
			+ '  ·  Black: ' + (deployLockBlack ? 'ready ✓' : 'arranging…');
		el.classList.remove('hidden');
		return;
	}
	// player view: surface only the opponent's status
	const opponentLocked = deployIsWhite ? deployLockBlack : deployLockWhite;
	if (opponentLocked) {
		el.textContent = 'Opponent locked in ✓';
		el.classList.remove('hidden');
	} else {
		el.classList.add('hidden');
	}
};

const clearDeployLockIndicator = () => {
	deployLockWhite = false;
	deployLockBlack = false;
	const el = document.getElementById('deploy-opponent-status');
	if (el) {
		el.classList.add('hidden');
		el.textContent = '';
	}
};

const startDeployCountdown = (seconds) => {
	clearDeployCountdown();
	deployDeadline = Date.now() + seconds * 1000;
	const tick = () => {
		const remainMs = deployDeadline - Date.now();
		const el = document.getElementById('deploy-countdown');
		if (el) {
			el.textContent = Math.max(0, Math.ceil(remainMs / 1000)) + 's';
		}
		// auto-submit the current arrangement shortly before the server deadline
		// so an unconfirmed-but-present player keeps what they arranged
		if (!deployConfirmed && remainMs <= 2000) {
			confirmDeploy();
		}
		if (remainMs > 0) {
			deployTimer = setTimeout(tick, 250);
		}
	};
	tick();
};

const clearDeployCountdown = () => {
	if (deployTimer) {
		clearTimeout(deployTimer);
		deployTimer = null;
	}
};

/**
 * exitDeployMode is called when the post-deploy board state arrives (the reveal):
 * it fades out the "?" overlay(s), restores normal board interaction, and resets
 * the deploy controls for the next game. Handles both a player who arranged and a
 * spectator who watched the blind phase.
 */
const exitDeployMode = () => {
	if (!deployMode && !deploySpectating) {
		return;
	}
	const wasSpectating = deploySpectating;
	deployMode = false;
	deploySpectating = false;
	deployConfirmed = false;
	clearDeployCountdown();
	clearDeployDraft();

	const dq = document.getElementById('deploy-questions');
	const dqb = document.getElementById('deploy-questions-btm');
	const overlay = document.getElementById('deploy-overlay');
	overlay.classList.remove('deploy-show');
	// brief fade of the "?" cells while the revealed pieces render underneath
	dq.classList.add('deploy-reveal');
	dqb.classList.add('deploy-reveal');
	setTimeout(() => {
		dq.classList.remove('deploy-show', 'deploy-reveal');
		dqb.classList.remove('deploy-show', 'deploy-reveal');
	}, 500);

	// a spectator never had interaction to restore; a player regains normal play
	// (and octadground's auto-castle)
	if (!wasSpectating) {
		og.set({ autoCastle: true, draggable: { enabled: true }, selectable: { enabled: window.isMobile } });
	}

	// reset the card (a spectator overwrote its text) and controls for next time
	document.querySelector('.deploy-headline').textContent = 'Arrange your pieces';
	document.querySelector('.deploy-hint').textContent = 'Drag a piece onto another — or tap two squares — to swap, then confirm.';
	document.getElementById('deploy-countdown').classList.remove('hidden');
	document.getElementById('deploy-confirm').classList.remove('hidden');
	document.getElementById('deploy-waiting').classList.add('hidden');
	clearDeployLockIndicator();
};

// ---- move list + review navigation ----

const moveListEl = document.getElementById('moveList');
const navFirstBtn = document.getElementById('nav-first');
const navPrevBtn = document.getElementById('nav-prev');
const navNextBtn = document.getElementById('nav-next');
const navLastBtn = document.getElementById('nav-last');

/**
 * playerColor returns the viewer's board orientation color, defaulting to white
 * for spectators (who match neither player id).
 */
const playerColor = () => (playerWhite ? 'white' : 'black');

/**
 * escapeHtml defensively escapes text destined for innerHTML. SAN strings are
 * server-generated and safe, but this keeps the move-list render injection-proof.
 */
const escapeHtml = (s) => String(s).replace(/[&<>"']/g, (c) => ({
	'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
}[c]));

/**
 * lastMoveHighlight returns the [from, to] square pair for the move leading into
 * the given ply (ply >= 1), for octadground's last-move highlight, or [] at the
 * start position.
 */
const lastMoveHighlight = (ply) => {
	const uoi = ply > 0 ? history.uois[ply - 1] : null;
	return uoi ? [uoi.substring(0, 2), uoi.substring(2, 4)] : [];
};

/**
 * checkAtPly reports whether the position at the given ply leaves the side to
 * move in check. Per-ply check state isn't sent, but it's implied by the SAN of
 * the move leading into the ply (server SANs carry '+' for check and '#' for
 * checkmate). The start position (ply 0) has no preceding move and is never a
 * check.
 * @param ply - ply index into history.ofens (0 = start)
 */
const checkAtPly = (ply) => {
	const san = ply > 0 ? history.sans[ply - 1] : null;
	return !!san && (san.includes('+') || san.includes('#'));
};

/**
 * renderReviewPosition shows a past position on the board (non-interactive) from
 * the server-provided OFEN history, with the last-move highlight and, when the
 * move into this ply gave check, the checked-king indicator.
 * @param ply - ply index into history.ofens (0 = start)
 */
const renderReviewPosition = (ply) => {
	const ofen = history.ofens[ply];
	if (!ofen) {
		return;
	}
	const parts = ofen.split(' ');
	og.set({
		orientation: playerColor(),
		ofen: parts[0],
		lastMove: lastMoveHighlight(ply),
		turnColor: parts[1] === 'w' ? 'white' : 'black',
		// check: true flags the side-to-move's king, which is the side in check
		check: checkAtPly(ply),
		movable: { free: false, dests: new Map(), color: undefined },
	});
};

/**
 * goToPly moves the on-screen board to the given ply, clamped to
 * [0, currentPly]. Landing on the live tip restores the authoritative live board
 * (dests, turn, interactivity) from the cached last live message; any earlier
 * ply renders a read-only historical position.
 * @param ply - target ply
 */
const goToPly = (ply) => {
	// navigation is meaningless during the blind deploy phase
	if (deployMode || deploySpectating) {
		return;
	}
	if (ply < 0) {
		ply = 0;
	}
	if (ply > currentPly) {
		ply = currentPly;
	}
	const moved = ply !== viewPly;
	viewPly = ply;
	followingLive = (ply === currentPly);
	if (followingLive) {
		if (lastLiveMessage) {
			renderLivePosition(lastLiveMessage, lastLiveMessage.d.o.split(' '));
		}
	} else {
		renderReviewPosition(ply);
	}
	renderMoveList();
	// sound the move we traversed onto — the move leading into the ply we landed
	// on, which is also the last-move the board now highlights. Only when the ply
	// actually changed, so a clamped no-op (e.g. ▶ at the live tip) stays silent.
	if (moved) {
		playNavSound(ply > 0 ? history.sans[ply - 1] : null);
	}
};

/**
 * updateNavButtons enables/disables the ⏮◀▶⏭ controls based on the viewed ply.
 */
const updateNavButtons = () => {
	const atStart = viewPly <= 0;
	const atLive = viewPly >= currentPly;
	if (navFirstBtn) { navFirstBtn.disabled = atStart; }
	if (navPrevBtn) { navPrevBtn.disabled = atStart; }
	if (navNextBtn) { navNextBtn.disabled = atLive; }
	if (navLastBtn) { navLastBtn.disabled = atLive; }
};

/**
 * renderMoveList rebuilds the move-list panel from the SAN history, one row per
 * full move (number, white, black). The cell at the viewed ply is highlighted
 * and scrolled into view; ply 0 (start) highlights the panel via .at-start.
 */
const renderMoveList = () => {
	if (!moveListEl) {
		return;
	}
	const sans = history.sans || [];
	let html = '';
	for (let i = 0; i < sans.length; i += 2) {
		const num = (i / 2) + 1;
		const wPly = i + 1;
		const bPly = i + 2;
		html += '<div class="move-row">';
		html += '<span class="move-num">' + num + '.</span>';
		html += '<span class="move' + (viewPly === wPly ? ' active' : '')
			+ '" data-ply="' + wPly + '" role="listitem">' + escapeHtml(sans[i]) + '</span>';
		if (sans[i + 1] !== undefined) {
			html += '<span class="move' + (viewPly === bPly ? ' active' : '')
				+ '" data-ply="' + bPly + '" role="listitem">' + escapeHtml(sans[i + 1]) + '</span>';
		} else {
			html += '<span class="move move-empty"></span>';
		}
		html += '</div>';
	}
	moveListEl.innerHTML = html;
	moveListEl.classList.toggle('at-start', viewPly === 0);

	const active = moveListEl.querySelector('.move.active');
	if (active) {
		active.scrollIntoView({ block: 'nearest' });
	}
	updateNavButtons();
};

// nav buttons + clickable move rows
if (navFirstBtn) { navFirstBtn.addEventListener('click', () => goToPly(0)); }
if (navPrevBtn) { navPrevBtn.addEventListener('click', () => goToPly(viewPly - 1)); }
if (navNextBtn) { navNextBtn.addEventListener('click', () => goToPly(viewPly + 1)); }
if (navLastBtn) { navLastBtn.addEventListener('click', () => goToPly(currentPly)); }
if (moveListEl) {
	moveListEl.addEventListener('click', (e) => {
		const cell = e.target.closest('.move[data-ply]');
		if (!cell) {
			return;
		}
		const ply = parseInt(cell.getAttribute('data-ply'), 10);
		if (!isNaN(ply)) {
			goToPly(ply);
		}
	});
}

// keyboard navigation: ←/→ step one move, ↑/Home to start, ↓/End to live. Left
// alone while arranging a blind deploy, when typing in a field, or when a
// modifier key is held (so browser/OS shortcuts keep working).
document.addEventListener('keydown', (e) => {
	if (deployMode || deploySpectating) {
		return;
	}
	if (e.metaKey || e.ctrlKey || e.altKey || e.shiftKey) {
		return;
	}
	const el = document.activeElement;
	if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable)) {
		return;
	}
	switch (e.key) {
		case 'ArrowLeft':
			goToPly(viewPly - 1);
			break;
		case 'ArrowRight':
			goToPly(viewPly + 1);
			break;
		case 'ArrowUp':
		case 'Home':
			goToPly(0);
			break;
		case 'ArrowDown':
		case 'End':
			goToPly(currentPly);
			break;
		default:
			return;
	}
	e.preventDefault();
});

window.handlers.set(moveTag, handleMove);
window.handlers.set(gameOverTag, handleGameOver);
window.handlers.set(rematchUpdateTag, handleRematchUpdate);
window.handlers.set(drawOfferTag, handleDrawOffer);
window.handlers.set(deployTag, handleDeploy);
