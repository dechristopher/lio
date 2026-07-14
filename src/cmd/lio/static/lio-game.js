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
// gameResult is the PGN result token for the current game, set by the game-over
// message and reset on each new game. Read by the analysis-mode copy-PGN button
// (which is only visible once the game is over, so it's always final when read).
let gameResult = '*';
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
// deploySubmitAcked: the server confirmed receipt of OUR arrangement. Until it
// does, sendDeploySubmit keeps resending on deploySubmitRetryTimer so a confirm
// lost to a connectivity blip recovers on its own (a move's pending/ack analogue).
let deploySubmitAcked = false;
let deploySubmitRetryTimer = null;
// deployPriorGameIDs holds every game id known to predate the current deploy
// phase, captured at phase entry: the pre-deploy game the server names in the
// deploy message itself (d.i — during a rematch this is the fresh placeholder
// the room swapped in, a game this client may never have seen a snapshot of)
// plus whatever id we were tracking (currentGameID — during a rematch, the
// finished game). While in deploy mode, any board state whose id is NOT in
// this set can only be the deployed game, so it is treated as the reveal even
// when the single gs=true broadcast was missed — including when the set is
// empty (a client whose only pre-deploy server message was a deploy-state
// message, the norm for game 1 of a bot room; the plain gid !== currentGameID
// test in handleMove requires a non-null prior id and cannot recognize that
// case — see DEPLOY_REMATCH_RACES.md). Board states carrying any id in the
// set are stale pre-deploy positions and stay dropped.
let deployPriorGameIDs = [];

window.addEventListener('load', () => {
	if (window.ws) {
		return false;
	}
	connect();
	return true;
});


// Each sound is ogg-first, mp3-second so browsers without Ogg Vorbis (older iOS
// Safari) fall back to the mp3 rather than staying silent — see the note in
// lio.js where confirmation/notification are defined.
window.moveSound = new Howl({
	src: ["/res/sfx/move.ogg", "/res/sfx/move.mp3"],
	preload: true,
	// Web Audio (Howler's default), NOT html5. This is the most-played sound
	// (every non-capturing, non-checking move, plus deploy swaps), and it must
	// ride the same single Web Audio unlock as capSound/checkSound/confirmation/
	// notification: Howler resumes one AudioContext on the first user gesture and
	// then every Web Audio sound plays reliably. html5:true instead routes this
	// one sound through the pooled <audio> path, whose separate per-element
	// unlock is unreliable on mobile (notably Android Firefox) — producing moves
	// that render silently while captures/checks (Web Audio) still sound. The
	// flag was a leftover from a since-removed autoplay:true; no autoplay means
	// the pool-exhaustion concern it guarded against no longer applies.
	volume: 0.75
});

window.capSound = new Howl({
	src: ["/res/sfx/capture.ogg", "/res/sfx/capture.mp3"],
	preload: true,
	volume: 0.9
});

window.checkSound = new Howl({
	src: ["/res/sfx/check.ogg", "/res/sfx/check.mp3"],
	preload: true,
	volume: 0.9
});

// Notification cues for the opponent's non-move actions, kept deliberately
// subtle (soft synthesized ticks) so they read as a nudge, not a board event.
// drawSound is a bright double tick (a decision is being asked of you);
// rematchSound is a single warm tick (a lighter "let's go again" prompt). Both
// play only on the rising edge of the opponent's action — never on the resync
// poll that re-delivers the standing offer/agreement (see handleDrawOffer and
// showOpponentRematchRequest).
window.drawSound = new Howl({
	src: ["/res/sfx/tick-draw.ogg", "/res/sfx/tick-draw.mp3"],
	preload: true,
	volume: 0.6
});

window.rematchSound = new Howl({
	src: ["/res/sfx/tick-rematch.ogg", "/res/sfx/tick-rematch.mp3"],
	preload: true,
	volume: 0.6
});

// Watch-only (spectator) mode, decided server-side (view/components.templ sets
// data-spectator on the board container for viewers holding no seat). While
// spectating, the board never accepts input — no movable dests, no dragging,
// no premoves — and every player-only affordance below is skipped. The server
// independently drops game-affecting frames from spectator sockets, so this
// flag is presentation-only: forging it can't affect the real match.
const isSpec = document.getElementById('gcon-xx').dataset.spectator === 'true';

// The anchored player's id (spectators only; see RoomTemplatePayload.AnchorID).
// The spectator view pins this player to the bottom of the board and the bottom
// scoreboard/timeline row across the color swaps between games of a match — the
// board flips instead (the same anchoring the TV grid uses). The server renders
// the initial orientation class from the anchor's current color.
const anchorId = document.getElementById('gcon-xx').dataset.anchor || '';

// create game board, oriented by the server-rendered class: the player's own
// color, or the anchored player's current color for a spectator (whose
// interaction is all off — mirrors enterDeploySpectatorMode's config; drawable
// shapes stay usable).
let og = Octadground(document.getElementById('game'), {
	ofen: 'ppkn/4/4/NKPP', // set initial board state to prevent brief period of missing pieces
	orientation: document.getElementById('gcon-xx').classList.contains('b') ? 'black' : 'white',
	highlight: {
		lastMove: true,
		check: true,
	},
	movable: isSpec ? {
		free: false,
		color: undefined,
		dests: new Map(),
	} : {
		free: false,
		color: document.getElementById('gcon-xx').classList.contains('w') ? 'white' : 'black'
	},
	draggable: {
		enabled: !isSpec,
	},
	premovable: {
		enabled: !isSpec,
	},
	selectable: {
		enabled: !isSpec && window.isMobile,
	},
	events: {
		move: (orig, dest, capturedPiece) => {
			if (isSpec) {
				return; // never reachable with input disabled; belt and braces
			}
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
	// final chokepoint: a spectator's client never puts a move on the wire,
	// whatever input path produced it (the server would drop it anyway)
	if (isSpec) {
		return;
	}
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
 * Returns true if the bottom-of-board player is playing white. For a player
 * that is themselves; for a spectator it is the anchored player (data-anchor),
 * whose id substitutes for the viewer's own uid — so the anchored player keeps
 * the bottom while the board flips across between-game color swaps. This one
 * convention orients everything downstream of it (clock mapping, active-clock
 * highlight, score chips, the match timeline rows, and review orientation) to
 * bottom = the anchored seat. A bot seat has no id, so in a bot game the
 * anchored human reads correctly from d.w matching (bot as white sends no w).
 * @param message - move message
 * @returns {boolean} is white
 */
const isPlayerWhite = (message) => {
	if (isSpec) {
		// no anchor (degenerate un-started room): fall back to white-on-bottom
		return anchorId === '' || message.d.w === anchorId;
	}
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

// whether the opponent in this game is the engine/bot (only meaningful for a
// player — the bot always sits on the opponent clock for them), plus the clock
// element carrying the bot seat. Spectator clocks are anchored by player with
// the human pinned to the bottom, so for them too a bot only ever sits on the
// top clock; the general two-element query stays as belt and braces. The
// "thinking" indicator lives on whichever clock that is.
const clockOpponentEl = document.getElementById('clockOpponent');
const clockPlayerEl = document.getElementById('clockPlayer');
const opponentIsBot = !!clockOpponentEl && clockOpponentEl.dataset.bot === 'true';
const botClockEl = document.querySelector('#clockOpponent[data-bot="true"], #clockPlayer[data-bot="true"]');
const botClockOnBottom = !!botClockEl && botClockEl.id === 'clockPlayer';
const thinkingEl = botClockEl ? botClockEl.querySelector('.thinking') : null;

// ---- seat presence indicators (the dot / tinted CPU glyph in each clock) ----
// Crowd messages report presence by color; the bottom clock is the local
// player's (or, spectating, the anchored player's) seat. Colors swap between
// games of a match, so the mapping follows the live playerWhite cache rather
// than the server-rendered orientation class, which goes stale after a swap.
const bottomClockIsWhite = () => playerWhite;

const setPresence = (el, on) => {
	if (el) {
		el.classList.toggle('presence-on', !!on);
	}
};

/**
 * updatePresence reflects per-seat connection state on the clock presence
 * indicators. A bot seat never holds a socket (the server always reports it
 * disconnected), but the bot lives server-side: it is present whenever this
 * client's room link is — and crowd messages only arrive over a live link, so
 * a bot seat simply reads connected here (clearPresence greys it on close).
 * @param whiteConnected - white seat holds a connected socket
 * @param blackConnected - black seat holds a connected socket
 */
const updatePresence = (whiteConnected, blackConnected) => {
	const bottomOn = bottomClockIsWhite() ? whiteConnected : blackConnected;
	const topOn = bottomClockIsWhite() ? blackConnected : whiteConnected;
	setPresence(clockPlayerEl, (clockPlayerEl && clockPlayerEl.dataset.bot === 'true') || bottomOn);
	setPresence(clockOpponentEl, (clockOpponentEl && clockOpponentEl.dataset.bot === 'true') || topOn);
};

/**
 * clearPresence greys every presence indicator (bots included): with our own
 * socket down we can't know anyone's state, and a closed room has no bot.
 */
const clearPresence = () => {
	setPresence(clockPlayerEl, false);
	setPresence(clockOpponentEl, false);
};

// ---- material-difference icons (the piece stacks beside each clock name) ----
// Computed from the *position* rather than capture events (lichess-style), so
// promotions count correctly: per piece type, diff = white count - black count,
// and each seat shows icons for the types it is up, rendered in the opponent's
// color (the pieces it effectively captured), plus a +N point score on the
// leading seat. Standard point values (the engine's material scale / 10).
const materialRoles = { p: 'pawn', n: 'knight', b: 'bishop', r: 'rook', q: 'queen' };
const materialOrder = ['pawn', 'knight', 'bishop', 'rook', 'queen'];
const materialValues = { pawn: 1, knight: 3, bishop: 3, rook: 5, queen: 9 };

const clockMaterialPlayerEl = clockPlayerEl ? clockPlayerEl.querySelector('.clockMaterial') : null;
const clockMaterialOpponentEl = clockOpponentEl ? clockOpponentEl.querySelector('.clockMaterial') : null;

/**
 * materialHTML renders one seat's material advantage: a .mat-group of stacked
 * <piece> sprites per piece type it is up (the sprite art comes from the active
 * piece-set theme CSS), then the +N score when this seat leads on points.
 * @param up - map of piece type -> count this seat is up
 * @param iconColor - sprite color for the icons (the opponent's color)
 * @param score - this seat's net point lead (rendered only when > 0)
 * @returns {string} HTML for the seat's .clockMaterial span
 */
const materialHTML = (up, iconColor, score) => {
	let html = '';
	for (const role of materialOrder) {
		const n = up[role] || 0;
		if (n === 0) {
			continue;
		}
		html += '<span class="mat-group">'
			+ ('<piece class="' + role + ' ' + iconColor + '"></piece>').repeat(n)
			+ '</span>';
	}
	if (score > 0) {
		html += '<span class="mat-score">+' + score + '</span>';
	}
	return html;
};

/**
 * updateMaterial rebuilds both clocks' material-difference indicators from a
 * board position. Called with the *viewed* position (live tip or a reviewed
 * ply), so navigating the move history updates the icons alongside the board.
 * The bottom clock is the local (or anchored) player's seat; the playerWhite
 * cache maps it to a color, following the between-game swaps of a match.
 * @param boardOfen - board part of an OFEN (piece placement only)
 */
const updateMaterial = (boardOfen) => {
	if (!clockMaterialPlayerEl || !clockMaterialOpponentEl) {
		return;
	}
	const white = {};
	const black = {};
	for (const ch of boardOfen) {
		const role = materialRoles[ch.toLowerCase()];
		if (!role) {
			continue; // kings, empty-square digits, rank separators
		}
		const counts = ch === ch.toLowerCase() ? black : white;
		counts[role] = (counts[role] || 0) + 1;
	}
	const whiteUp = {};
	const blackUp = {};
	let score = 0; // white-positive net point lead
	for (const role of materialOrder) {
		const d = (white[role] || 0) - (black[role] || 0);
		if (d > 0) {
			whiteUp[role] = d;
		} else if (d < 0) {
			blackUp[role] = -d;
		}
		score += d * materialValues[role];
	}
	const whiteHTML = materialHTML(whiteUp, 'black', score);
	const blackHTML = materialHTML(blackUp, 'white', -score);
	clockMaterialPlayerEl.innerHTML = bottomClockIsWhite() ? whiteHTML : blackHTML;
	clockMaterialOpponentEl.innerHTML = bottomClockIsWhite() ? blackHTML : whiteHTML;
};

/**
 * clearMaterial empties both material indicators (the :empty CSS collapses
 * them). Used entering the blind deploy phase: the previous game's diff is
 * stale, and deploy-phase positions are hidden/partial by design.
 */
const clearMaterial = () => {
	if (clockMaterialPlayerEl) {
		clockMaterialPlayerEl.innerHTML = '';
	}
	if (clockMaterialOpponentEl) {
		clockMaterialOpponentEl.innerHTML = '';
	}
};

/**
 * Show or hide the engine "thinking" indicator. No-op unless a seat is the
 * engine, so it never appears in human-vs-human games.
 * @param on - whether the engine is currently thinking
 */
const setThinking = (on) => {
	if (!thinkingEl) {
		return;
	}
	thinkingEl.classList.toggle('thinking-on', !!on);
};

/**
 * Update the thinking indicator from a board-state message: the engine is
 * thinking whenever the game is still ongoing and it is the bot's turn. The
 * bottom clock is "the player" in isPlayerTurn terms (the anchored player, for
 * a spectator),
 * so the bot's turn is the bottom clock's turn when the bot sits there.
 * @param message - move message
 * @param ofenParts - OFEN parts array
 */
const updateThinking = (message, ofenParts) => {
	// a non-empty legal-move set means the game is still in progress; an empty
	// set means checkmate/stalemate, where nobody is "thinking"
	const gameOngoing = !!message.d.v && Object.keys(message.d.v).length > 0;
	const botTurn = botClockOnBottom
		? isPlayerTurn(message, ofenParts)
		: !isPlayerTurn(message, ofenParts);
	// gameOver guards finished-but-playable positions (resignation, flag):
	// their snapshots still carry legal moves, but nobody is thinking anymore
	setThinking(!gameOver && gameOngoing && botTurn);
};

// game-end result overlay elements and the player's color, cached from move
// messages (the game-over message reuses the `w` key for the winner, so the
// player's color can't be derived from it)
const resultOverlayEl = document.getElementById('result-overlay');
const resultCardEl = resultOverlayEl
	? resultOverlayEl.querySelector('.result-card') : null;
const resultHeadlineEl = document.getElementById('result-headline');
const resultReasonEl = document.getElementById('result-reason');
const resultScoreEl = document.getElementById('result-score');
const resultCountdownEl = document.getElementById('result-countdown');
const rematchBtn = document.getElementById('result-rematch');
const homeBtn = document.getElementById('result-home');
// beat between the final position rendering and the result card fading in, so
// the deciding move registers on the board before the card covers it
const resultShowDelayMs = 1000;
// must outlast the .result-closing exit animations in app.css (160ms)
const resultCloseMs = 180;
// timer id while the result card's delayed fade-in is pending, else null; a
// pending show is treated everywhere like a showing card (see
// resultShowingOrPending), and every teardown/dismiss path cancels it
let resultShowTimer = null;

// beat between an opponent's move landing and the view snapping back up to the
// board on mobile (see snapViewToBoard)
const boardSnapDelayMs = 1000;
// timer id while an opponent-move snap is pending, else null; a game-over
// supersedes it (the result card's fade-in snaps instead)
let boardSnapTimer = null;

/**
 * snapViewToBoard smooth-scrolls the page back to the top so the board is
 * fully in view. Only meaningful on the single-column (mobile) layout, where
 * the moves panel and controls can pull the board off-screen — a no-op on the
 * two-column desktop layout (the 899px test mirrors the CSS breakpoint) and
 * for users preferring reduced motion the scroll is instant.
 */
const snapViewToBoard = () => {
	if (!window.matchMedia('(max-width: 899px)').matches) {
		return;
	}
	const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	window.scrollTo({ top: 0, behavior: reduced ? 'auto' : 'smooth' });
};
// endgame annotation: the mid-board pill naming the finished game's result
// while its final position is on the board (see updateEndAnnotation)
const endAnnotationEl = document.getElementById('end-annotation');
let endAnnotationText = '';
// whether the bottom-of-board seat currently holds white (see isPlayerWhite).
// Seeded from the server-rendered orientation class so presence/clock mapping
// is right before the first board message, then re-cached from every board
// message (colors swap between games of a match).
let playerWhite = !document.getElementById('gcon-xx').classList.contains('b');
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
// the room grid also carries an .analyzing class while the game is over, which
// drives the compact mobile analysis layout (see app.css "analysis mode")
const gameGridEl = document.querySelector('.game-grid');
const resignBtn = document.getElementById('btn-resign');
const drawBtn = document.getElementById('btn-draw');
const railRematchBtn = document.getElementById('btn-rematch');

// two-step resign confirm state, and draw-offer state mirrored from the server's
// 'do' broadcasts (reset on each new game / whenever a move supersedes the offer)
let resignArmed = false;       // resign button is showing its confirm prompt
let resignArmTimer = null;     // auto-disarm timeout for the confirm prompt
let drawOfferedByMe = false;   // we have a standing draw offer out
let drawOfferedByOpp = false;  // the opponent has offered us a draw to accept

// tracks whether the "opponent wants a rematch" cue has already sounded for the
// current standing request, so the resync poll (which re-invokes
// showOpponentRematchRequest every tick) chimes only once. Reset when the
// request note is hidden (new game / teardown / our own commit).
let opponentRematchNotified = false;

// the rematch buttons (overlay + rail) driven together as a set; capture each
// one's idle label once so pending/disabled states can restore it. A spectator's
// buttons render permanently disabled and are left out of the set entirely, so
// none of the shared state helpers (default/pending/wants) can re-enable them.
const rematchButtons = isSpec ? [] : [rematchBtn, railRematchBtn].filter(Boolean);
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
// Once a race-to match is decided the same agreement flow starts a fresh match,
// so the action reads "New match" while that result is showing. Swapping the
// captured default label (rather than the live innerHTML) keeps every later
// state restore (default / disabled) consistent; showResult sets the mode for
// each result, so a following classic game-over reads "Rematch" again.
const setRematchButtonsNewMatch = (newMatch) => rematchButtons.forEach((b) => {
	if (!b.dataset.rematchLabel) { b.dataset.rematchLabel = b.dataset.defaultLabel; }
	b.dataset.defaultLabel = newMatch
		? b.dataset.rematchLabel.replace('Rematch', 'New match')
		: b.dataset.rematchLabel;
});

/**
 * setControlsMode swaps the rail control set between live play (Resign / Draw)
 * and game-over (Rematch). Toggling .controls-over does the visual swap; the
 * play-control state is reset either way so a stale confirm/offer never lingers.
 * The same over/live signal drives .analyzing on the game grid, which switches
 * the single-column (mobile) layout into its compact analysis arrangement.
 * @param over - true once the game is over (show Rematch), false during play
 */
const setControlsMode = (over) => {
	if (gameControlsEl) {
		gameControlsEl.classList.toggle('controls-over', over);
	}
	if (gameGridEl) {
		gameGridEl.classList.toggle('analyzing', over);
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
	// a spectator's draw button is permanently disabled; the reset below would
	// re-enable it (this runs from handleMove whenever a move lands)
	if (isSpec) {
		return;
	}
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
 * resultSummary builds the short result line for the endgame annotation
 * ("White wins by checkmate", "Draw by agreement"). Deliberately by color,
 * not "You win" — the same line reads correctly for players and spectators.
 * @param d - game over payload data
 * @returns {string} one-line result summary
 */
const resultSummary = (d) => {
	if (d.r === 'abandoned') {
		return 'Match over';
	}
	const who = d.w === 'd' ? 'Draw:' : (d.w === 'w' ? 'White wins:' : 'Black wins:');
	const method = resultReasons[d.r] || '';
	return method ? `${who} ${method}` : who;
};

/**
 * resultShowingOrPending reports whether the result overlay is up OR its
 * delayed fade-in is armed — states that everything asking "is the result
 * card telling the story?" must treat identically, or a game-over repeat /
 * rematch update / annotation toggle landing inside the one-beat delay
 * misbehaves.
 */
const resultShowingOrPending = () => !!resultOverlayEl
	&& (resultOverlayEl.classList.contains('result-show')
		|| resultShowTimer !== null);

/**
 * updateEndAnnotation shows/hides the mid-board result pill: visible only when
 * the game is over, the board is showing its final position (the live tip),
 * and the result card isn't already telling the story (never shown, or
 * dismissed for analysis). Called from every path that changes one of those
 * inputs: game over, ply navigation, card dismiss/restore, and new-game reset.
 */
const updateEndAnnotation = () => {
	if (!endAnnotationEl) {
		return;
	}
	const cardShowing = resultShowingOrPending()
		&& !resultOverlayEl.classList.contains('result-dismissed');
	const show = gameOver && !!endAnnotationText
		&& viewPly === currentPly && !cardShowing;
	endAnnotationEl.textContent = endAnnotationText;
	endAnnotationEl.classList.toggle('ea-show', show);
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

/**
 * Countdown label for an undecided race-to match's interlude: the next game
 * starts automatically when it lapses, no action needed from either player.
 */
const nextGameLabel = (remaining) => {
	if (remaining <= 0) {
		return 'Starting&hellip;';
	}
	return `Next game in ${remaining}s`;
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

// Deploy-phase resync safety net, the deploy analogue of the rematch poll
// above. After confirming an arrangement (or while spectating the blind
// phase) the client is waiting on a single server broadcast — the gs=true
// reveal — with nothing else scheduled to pull it forward if that one message
// is lost (mobile backgrounding half-opens the socket; a stalled write or
// full send buffer drops the connection server-side with the reveal still
// queued). While waiting, poll a:0: during the live phase the server answers
// with a DeployStateMessage (harmless re-entry refresh, and it resends a
// confirm the server never received); after the phase it answers with the
// deployed game's board state, which handleMove now recognizes as the reveal
// via deployStartGameID. Stopped by exitDeployMode and on room teardown. The
// happy path (reveal arrives within ~1s of the second confirm) sends zero
// extra traffic.
let deployResyncTimer = null;

const startDeployResync = () => {
	stopDeployResync();
	deployResyncTimer = setInterval(() => {
		sendBoardUpdateRequest();
	}, rematchResyncIntervalMs);
};

const stopDeployResync = () => {
	if (deployResyncTimer !== null) {
		clearInterval(deployResyncTimer);
		deployResyncTimer = null;
	}
};

/**
 * Hide the game-end result overlay and reset its rematch button and countdown.
 */
const hideResult = () => {
	// cancel a fade-in still waiting out its one-beat delay
	if (resultShowTimer !== null) {
		clearTimeout(resultShowTimer);
		resultShowTimer = null;
	}
	if (resultOverlayEl) {
		resultOverlayEl.classList.remove('result-show');
		// clear any "dismissed for analysis" state so the next game's overlay is
		// clean, and any in-flight dismiss fade-out (its timeout checks
		// result-closing and stands down)
		resultOverlayEl.classList.remove('result-dismissed');
		resultOverlayEl.classList.remove('result-closing');
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
	// the finished game's context is gone with it
	endAnnotationText = '';
	updateEndAnnotation();
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
	// chime once per standing request: this runs on every resync poll tick via
	// reconcileRematchAgreement, so gate playback on the not-yet-notified edge
	if (!opponentRematchNotified) {
		opponentRematchNotified = true;
		window.rematchSound.play();
	}
};

const hideOpponentRematchRequest = () => {
	const note = document.getElementById('result-note');
	if (note) {
		note.classList.add('hidden');
		note.textContent = '';
	}
	clearRematchButtonsWants();
	// the request is gone; re-arm the cue for the next standing request
	opponentRematchNotified = false;
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
	// a previous game in this room, and re-enable the rematch action (labeled
	// "New match" when this result decided a race-to match)
	rematchRequested = false;
	opponentLeft = false;
	hideOpponentRematchRequest();
	setRematchButtonsNewMatch(!!message.d.mo);
	setRematchButtonsDefault();

	const winner = message.d.w; // "w", "b", or "d"

	let outcome, headline;
	if (message.d.r === 'abandoned') {
		// abandonment closes the room; report it neutrally rather than as a draw
		outcome = 'draw';
		headline = 'Match over';
	} else if (message.d.mo && message.d.sc) {
		// the race is decided: headline the match, not the final game — which
		// may itself have been a draw that lifted the leader to the target. The
		// winner is the score leader (a decided match can't be tied); the method
		// subtitle below still describes the deciding game.
		const w = message.d.sc.w, b = message.d.sc.b;
		if (isSpec) {
			outcome = 'draw';
			headline = w > b ? 'White wins the match' : 'Black wins the match';
		} else {
			const mine = playerWhite ? w : b;
			const theirs = playerWhite ? b : w;
			outcome = mine > theirs ? 'win' : 'loss';
			headline = outcome === 'win' ? 'You win the match' : 'You lose the match';
		}
	} else if (winner === 'd') {
		outcome = 'draw';
		headline = 'Draw';
	} else if (isSpec) {
		// a spectator has no side to win or lose; report the result by color,
		// styled neutrally
		outcome = 'draw';
		headline = winner === 'w' ? 'White wins' : 'Black wins';
	} else if ((winner === 'w' && playerWhite) || (winner === 'b' && !playerWhite)) {
		outcome = 'win';
		headline = 'You win';
	} else {
		outcome = 'loss';
		headline = 'You lose';
	}

	// a rematch is impossible once the whole room is over (abandon / match end),
	// and there is nothing to click between games of an undecided race-to match
	// (they auto-advance): hide the overlay's button and disable the rail's
	const roomOver = !!message.d.o;
	const midMatch = !!message.d.rt && !message.d.mo && !roomOver;
	if (rematchBtn) {
		rematchBtn.style.display = (roomOver || midMatch) ? 'none' : '';
	}
	if (railRematchBtn && (roomOver || midMatch)) {
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

	// mid-match the next game auto-starts: count the interlude down (ng) and
	// start the resync poll unconditionally — the next game is announced by the
	// same single-shot broadcasts as a rematch, and no click arms the poll here.
	// It stops the moment the next game arrives (hideResult / enterDeployMode).
	// Otherwise human games tick the manual-rematch window (rw) down to the
	// room closing; bot games are not time-boxed and carry no countdown (the
	// finished room stays open for review + manual rematch), so a missing
	// ng/rw just clears the countdown.
	if (message.d.ng) {
		startCountdown(message.d.ng, nextGameLabel);
		startRematchResync();
	} else if (message.d.rw) {
		startCountdown(message.d.rw, rematchWindowLabel);
	} else {
		stopCountdown();
	}

	// one beat after the final position lands, fade the card in from the
	// direction of the deciding move. hideResult cancels this on teardown;
	// dismissResultForAnalysis converts a pending show straight into the
	// dismissed state so review navigation is never interrupted.
	setResultEntryVector();
	if (resultShowTimer !== null) {
		clearTimeout(resultShowTimer);
	}
	// a mating/flagging final move may have armed its own snap a moment ago;
	// the card's appearance takes over the scroll so they can't double-fire
	if (boardSnapTimer !== null) {
		clearTimeout(boardSnapTimer);
		boardSnapTimer = null;
	}
	resultShowTimer = setTimeout(() => {
		resultShowTimer = null;
		resultOverlayEl.classList.add('result-show');
		// the card is only useful on screen: on mobile the player may be
		// scrolled down at the moves panel when the game ends
		snapViewToBoard();
	}, resultShowDelayMs);
};

/**
 * setResultEntryVector aims the result card's pop-in (and dismiss fade-out)
 * along the vector from the board center to the final move's destination
 * square, in the player's orientation, so the card reads as emanating from
 * the square that decided the game. Endings with no move on record (e.g. an
 * abandoned game before the first move) get a random diagonal instead. The
 * offsets feed the resultPop/resultPopOut keyframes via CSS custom properties.
 */
const setResultEntryVector = () => {
	if (!resultCardEl) {
		return;
	}
	let dx, dy;
	const uoi = history.uois[history.uois.length - 1];
	if (uoi && uoi.length >= 4) {
		// destination square's offset from the board center, in board halves:
		// files a–d run left to right and ranks 1–4 bottom to top for white;
		// both axes invert for a black-oriented board
		dx = (uoi.charCodeAt(2) - 97 - 1.5) / 1.5;
		dy = (1.5 - (uoi.charCodeAt(3) - 49)) / 1.5;
		if (!playerWhite) {
			dx = -dx;
			dy = -dy;
		}
	} else {
		dx = Math.random() < 0.5 ? -1 : 1;
		dy = Math.random() < 0.5 ? -1 : 1;
	}
	resultCardEl.style.setProperty('--result-dx', `${(dx * 22).toFixed(1)}px`);
	resultCardEl.style.setProperty('--result-dy', `${(dy * 22).toFixed(1)}px`);
};

/**
 * postNavigate performs a full-page POST to url by submitting a transient form,
 * then follows the server's redirect. Used for state-changing navigations (e.g.
 * bot-rematch room creation) that must be POSTs, not GETs — a plain
 * location.href would be a GET the server's CSRF guard rejects. Query params in
 * url are preserved in the form action (read server-side via c.Query); the form
 * carries no body.
 * @param url - the POST target, may include a query string
 */
const postNavigate = (url) => {
	const form = document.createElement('form');
	form.method = 'POST';
	form.action = url;
	document.body.appendChild(form);
	form.submit();
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
	// spectators hold no rematch action (buttons render disabled and unwired;
	// this is the chokepoint guard behind them)
	if (isSpec) {
		return;
	}
	const socketDead = !window.ws || window.ws.readyState !== WebSocket.OPEN;
	const fallbackUrl = btn && btn.dataset.rematchUrl;
	if (socketDead && fallbackUrl) {
		// POST, not a GET navigation: /new/computer creates a room, so it is a
		// mutation the server's CSRF guard requires be a POST. The tc/color query
		// params in fallbackUrl are preserved (the server reads them via c.Query);
		// the server 302s to the fresh room.
		postNavigate(fallbackUrl);
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

/**
 * reconcileRematchAgreement syncs this client's rematch-request state with the
 * server's recorded per-seat agreement (rqw/rqb on every game-over payload —
 * including the repeats the resync poll receives and the reconnect game-over
 * state). It is the rematch analogue of the deploy-confirm reconcile: a click
 * lost on a half-open socket is detected (we believe we requested, the server
 * shows our seat un-agreed) and resent; an agreement the server holds but this
 * client forgot (a reload reset local state) restores the pending UI instead
 * of offering a stale button; and a missed single-shot 'ru' "wants a rematch"
 * signal is recovered from the same fields.
 * @param d - game-over payload data
 */
const reconcileRematchAgreement = (d) => {
	// spectators hold no rematch state; once the opponent has left the window
	// is closing and a rematch is off the table either way
	if (isSpec || opponentLeft) {
		return;
	}
	const mineAgreed = playerWhite ? !!d.rqw : !!d.rqb;
	const theirsAgreed = playerWhite ? !!d.rqb : !!d.rqw;
	if (rematchRequested && !mineAgreed) {
		// our click never reached the server: resend it. The poll repeats this
		// every tick until the recorded state confirms it landed.
		send(buildCommand("r", {rm: true}));
	} else if (!rematchRequested && mineAgreed) {
		// the server holds our agreement but this client doesn't remember
		// sending it (a reload/reconnect reset local state): restore the
		// pending "waiting" state and its resync poll
		rematchRequested = true;
		setRematchButtonsPending();
		hideOpponentRematchRequest();
		startRematchResync();
		refreshCountdownLabel();
	}
	if (theirsAgreed && !rematchRequested) {
		showOpponentRematchRequest();
	}
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
if (resignBtn && !isSpec) {
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
if (drawBtn && !isSpec) {
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
	// spectators have no draw button and no stake in a standing offer
	if (isSpec || gameOver) {
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
		// the opponent offered a draw: turn Draw into an accept affordance.
		// Cue the notification only on the rising edge — a reconnect/re-announce
		// can re-deliver a still-standing offer, and it must not re-chime.
		if (!drawOfferedByOpp) {
			window.drawSound.play();
		}
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

/**
 * dismissResultForAnalysis hides the showing result card and exposes the
 * restore button. Besides the "Analyze board" button, any review-navigation
 * input (arrow keys, nav buttons, move-list clicks) dismisses the card, so the
 * modal never blocks stepping through the finished game.
 */
const dismissResultForAnalysis = () => {
	if (!resultOverlayEl
		|| resultOverlayEl.classList.contains('result-dismissed')
		|| resultOverlayEl.classList.contains('result-closing')) {
		return;
	}
	if (resultShowTimer !== null) {
		// the card's delayed fade-in hasn't fired yet: cancel it and land
		// straight in the dismissed state so it never pops up over the review
		clearTimeout(resultShowTimer);
		resultShowTimer = null;
		resultOverlayEl.classList.add('result-show');
		resultOverlayEl.classList.add('result-dismissed');
		if (restoreResultBtn) {
			restoreResultBtn.classList.remove('hidden');
		}
		// the card never appeared; the board annotation takes over
		updateEndAnnotation();
		return;
	}
	if (!resultOverlayEl.classList.contains('result-show')) {
		return;
	}
	// fade the card back out toward the deciding move, then swap to the
	// click-through dismissed state once the exit animation has played
	resultOverlayEl.classList.add('result-closing');
	setTimeout(() => {
		// a new-game teardown (hideResult) may have raced the fade and already
		// cleared result-closing — stand down rather than resurrect state
		if (!resultOverlayEl.classList.contains('result-closing')) {
			return;
		}
		resultOverlayEl.classList.remove('result-closing');
		resultOverlayEl.classList.add('result-dismissed');
		if (restoreResultBtn) {
			restoreResultBtn.classList.remove('hidden');
		}
		// the card no longer names the result; the board annotation takes over
		updateEndAnnotation();
	}, resultCloseMs);
};

if (analyzeBtn) {
	analyzeBtn.addEventListener('click', dismissResultForAnalysis);
}
if (restoreResultBtn) {
	restoreResultBtn.addEventListener('click', () => {
		if (resultOverlayEl) {
			resultOverlayEl.classList.remove('result-dismissed');
		}
		restoreResultBtn.classList.add('hidden');
		// the card is back over the board; drop the redundant annotation
		updateEndAnnotation();
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
	// below, which all break across a game boundary. Outside the deploy phase,
	// the very first id after a page load is adopted silently (it describes the
	// game already in progress, not a boundary we crossed).
	const gid = message.d.i;
	// during the deploy phase the id is compared against the set of known
	// pre-deploy ids captured at phase entry (deployPriorGameIDs) rather than
	// currentGameID alone: the only board state that can follow a deploy phase
	// is the deployed game, so any id outside that set — including the first
	// id ever seen, when the set is empty — is the reveal, even when the
	// gs=true broadcast was missed. The plain currentGameID test below
	// requires a non-null prior id and so can never recognize the reveal for a
	// client whose only pre-deploy server message was a deploy-state message
	// (the norm for game 1 of a bot room).
	const inDeploy = deployMode || deploySpectating;
	const newGame = !!message.d.gs
		|| (!!gid && !!currentGameID && gid !== currentGameID)
		|| (!!gid && inDeploy && !deployPriorGameIDs.includes(gid));

	// while arranging or spectating the blind deploy phase, ignore stale pre-deploy
	// board states (r.game is still the previous position server-side); only a
	// new game's board state (the reveal, or any later snapshot of it if the
	// reveal was missed) ends the phase and renders pieces. The id is NOT
	// adopted on this drop: adopting it would make every later snapshot of the
	// deployed game read as the same game and never as new, permanently
	// wedging the client in deploy mode.
	if (inDeploy && !newGame) {
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

	if (gid) {
		currentGameID = gid;
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
			// position, so the move never arrived. Resend if it's still our turn
			// — unless the game has since ended (a move fired right at game end,
			// e.g. a premove on the final position, can never land; resending it
			// would just spin the reconcile loop against the finished room).
			reconciling = false;
			if (!gameOver && isPlayerTurn(message, ofenParts)) {
				resendPending();
			} else {
				clearPending();
			}
		}
	}

	// whether this message carries a genuinely new move (the ply advanced) —
	// captured before lastPly is overwritten; repeats/resync snapshots of the
	// same position never qualify
	const plyAdvanced = !newGame && serverPly > lastPly;

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
		gameResult = '*';
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
	} else if (!gameOver && !followingLive && message.d.m
		&& isPlayerParticipant(message) && isPlayerTurn(message, ofenParts)) {
		// snap a reviewing player back to the live board the moment it becomes
		// their turn to move, so they're never left unable to play. Never after
		// game over: the final position often has the player "to move" (mated,
		// flagged, or opponent resigned), and the server re-answers board queries
		// with that final state, which would repeatedly yank an analyzing player
		// off the ply they're reviewing.
		followingLive = true;
	}

	// an opponent's move just landed (the ply advanced and it's now our turn):
	// on mobile, ease the view back up to the board a beat later, so a player
	// who scrolled down to the moves panel isn't left below the fold while
	// their clock runs. Own moves never snap (the player is at the board), and
	// a game-ending move hands the scroll over to the result card (showResult
	// cancels this timer). The reviewing-player case is covered too: the
	// yank-back-to-live above runs first, and a non-participant never passes
	// isPlayerTurn's uid test.
	if (plyAdvanced && !gameOver && !isSpec && isPlayerTurn(message, ofenParts)) {
		if (boardSnapTimer !== null) {
			clearTimeout(boardSnapTimer);
		}
		boardSnapTimer = setTimeout(() => {
			boardSnapTimer = null;
			snapViewToBoard();
		}, boardSnapDelayMs);
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

	// perform pre-move if set (spectators can never set one — premovable is
	// disabled — but never even ask the board to play one for them)
	if (followingLive && !isSpec) {
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
		orientation: isPlayerWhite(message) ? 'white' : 'black',
		ofen: ofenParts[0],
		lastMove: getLastMove(message.d.m),
		turnColor: whiteToMove(ofenParts) ? "white" : "black",
		check: message.d.k,
		// a spectator's board never carries destinations or a movable color:
		// populating them from the uid test (which reads "black" for a viewer
		// matching neither player) would let a spectator drag the black pieces
		// around their own out-of-sync board. Premoves are re-asserted off on
		// every spectator render (players' premove state is left untouched).
		...(isSpec ? { premovable: { enabled: false } } : {}),
		movable: isSpec ? {
			free: false,
			dests: new Map(),
			color: undefined,
		} : {
			free: false,
			dests: gameOver ? new Map() : allMoves(message.d.v),
			color: message.d.w === getCookie('uid') ? 'white' : 'black',
		}
	});
	updateMaterial(ofenParts[0]);
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
	if (gameOver && message.d.o !== true && resultShowingOrPending()) {
		// defensively re-freeze the clocks: a repeat means the game is over, so
		// no interpolator may be left running whatever path armed it
		cancelAnimationFrame(frameId);
		if (message.d.rw) {
			startCountdown(message.d.rw, rematchWindowLabel);
		} else if (message.d.ng) {
			// a mid-match repeat (reconnect / poll): retime the auto-advance
			startCountdown(message.d.ng, nextGameLabel);
		}
		updateScore(message);
		// every repeat carries the server's recorded per-seat agreement; use it
		// to resend a lost click or surface a missed opponent request. Between
		// games of an undecided match there is no agreement to reconcile.
		if (!message.d.ng) {
			reconcileRematchAgreement(message.d);
		}
		return;
	}

	// whether this message is a room-cleanup notice arriving after the result was
	// already shown (the rematch/analysis window lapsed), rather than a fresh
	// game-over — captured before the flag is (re)set below
	const wasGameOver = gameOver;

	cancelAnimationFrame(frameId);
	document.getElementById("info").innerHTML = message.d.s;
	// don't replay the end sound for a room-cleanup notice while the player is
	// reviewing the finished game; the result it announces is long since heard
	if (!(wasGameOver && message.d.o === true && isAnalyzing())) {
		window.notification.play();
	}

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
		// record the PGN result token for the copy-PGN button; an abandonment
		// closes the room without a decided result, so it stays "*"
		if (message.d.r !== 'abandoned') {
			gameResult = message.d.w === 'w' ? '1-0'
				: message.d.w === 'b' ? '0-1' : '1/2-1/2';
		}
		showResult(message);
		// remember the result line for the mid-board analysis annotation (hidden
		// for now behind the result card; dismissing it hands over)
		endAnnotationText = resultSummary(message.d);
	}
	updateEndAnnotation();

	// disallow further moves
	og.set({
		movable: {
			dests: new Map()
		}
	})

	// update match score
	updateScore(message);

	// sync rematch state with the server's recorded agreements — a reconnect's
	// game-over state restores a pending request this client no longer
	// remembers (page reload) and a standing opponent request whose one-shot
	// 'ru' announcement was missed. Mid-match (ng) there is no agreement flow.
	if (message.d.o !== true && !message.d.ng) {
		reconcileRematchAgreement(message.d);
	}

	// if room over, decide whether to keep the player here or send them home
	if (message.d.o === true) {
		// no next game is coming; stop any post-rematch or deploy resync poll so
		// neither can outlive the room
		stopRematchResync();
		stopDeployResync();

		// A spectator always stays on the final position: the full move history
		// is client-held, so review keeps working after the room is gone. Stop
		// auto-reconnect so the client doesn't fight the deleted room.
		if (isSpec) {
			stopCountdown();
			if (resultCountdownEl) {
				resultCountdownEl.innerHTML = 'Room closed';
			}
			if (typeof window.lioStopReconnect === 'function') {
				window.lioStopReconnect();
			}
			return;
		}

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

		// A human-vs-human room that closed because no rematch was agreed (a bare
		// cleanup notice — no result attached — after the result was already
		// shown). Don't boot the player: a rematch is now impossible, so disable
		// it and drop the socket for good, but leave them free to analyze the
		// finished game.
		if (!opponentIsBot && wasGameOver && !message.d.w) {
			setRematchButtonsDisabled();
			stopCountdown();
			if (resultCountdownEl) {
				resultCountdownEl.innerHTML = 'Room closed';
			}
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

// ---- match score timeline ----
// One column per game of the room's match, one row per player, with the local
// player — or, spectating, the anchored player — always on the bottom row
// (their bottom-of-board point of view — the rows do NOT swap when the colors
// do between games; cell tints carry the color). Rebuilt in full from the
// history payload (d.h) of every score-bearing message via updateScore, so
// moves, game overs, and reconnect snapshots all refresh it and missed game
// boundaries self-heal. Markup shell lives in view/room.templ (#match-timeline).
const timelineEl = document.getElementById('match-timeline');
const tlOpp = timelineEl ? timelineEl.querySelector('#tl-row-opponent') : null;
const tlPly = timelineEl ? timelineEl.querySelector('#tl-row-player') : null;

// the two rows' game strips are separate overflow scrollers (the grid keeps
// their columns aligned), so mirror scrollLeft between them — a swipe on either
// row carries the other with it instead of shearing the columns apart. Writing
// an equal scrollLeft doesn't re-fire the scroll event, so this can't loop.
const tlStrips = timelineEl
	? Array.from(timelineEl.querySelectorAll('.tl-games')) : [];
tlStrips.forEach((strip) => {
	strip.addEventListener('scroll', () => {
		tlStrips.forEach((other) => {
			if (other !== strip && other.scrollLeft !== strip.scrollLeft) {
				other.scrollLeft = strip.scrollLeft;
			}
		});
	}, { passive: true });
});

// backend reason codes -> timeline win-method glyphs. Draw methods all render
// as '=' on both rows (the cell's ½ already says it was a draw); decisive
// methods mark only the winner's cell (see tlCell).
const tlGlyphs = {
	checkmate: '#',
	time: '⌛',
	resignation: '⚑',
	stalemate: '=',
	insufficient: '=',
	agreement: '=',
	repetition: '=',
	moverule: '=',
	abandoned: '×',
};

// 0.5 -> ½, 1.5 -> 1½, 2 -> 2 (cells and the name-side totals)
const tlPoints = (pts) => {
	const whole = Math.floor(pts);
	const half = pts - whole >= 0.5 ? '½' : '';
	return (whole || !half) ? `${whole}${half}` : half;
};

/**
 * tlCell builds one finished-game timeline cell for one player: their points,
 * the win-method glyph (winner's cell only; draws mark both), and the
 * white/black tint of the side they held that game.
 * @param pts - points this player earned (1, 0.5, or 0)
 * @param color - 'w'/'b', the side this player held that game
 * @param reason - backend method code (see resultReasons)
 * @param num - 1-based game number, for the tooltip
 * @returns {HTMLElement} the cell
 */
const tlCell = (pts, color, reason, num) => {
	const cell = document.createElement('span');
	const result = pts >= 1 ? 'win' : (pts >= 0.5 ? 'draw' : 'loss');
	cell.className = `tl-cell tl-${result} tl-${color === 'w' ? 'white' : 'black'}`;
	cell.setAttribute('role', 'listitem');

	const method = resultReasons[reason] || '';
	cell.title = `Game ${num}: ${result} ${method}`.trim();

	let html = tlPoints(pts);
	const glyph = tlGlyphs[reason];
	if (glyph && pts >= 0.5) {
		html += `<span class="tl-glyph">${glyph}</span>`;
	}
	cell.innerHTML = html;
	return cell;
};

/**
 * renderTimeline rebuilds the match timeline from a score-bearing message:
 * name-side totals, one column per finished game, plus a pulsing live column
 * for the game in progress (absent between games / once the room is over).
 * @param message - move/game-over message (d.sc present, d.h optional)
 */
const renderTimeline = (message) => {
	if (!tlOpp || !tlPly) {
		return;
	}

	// totals beside the names mirror the clock score chips
	const w = message.d.sc.w || 0;
	const b = message.d.sc.b || 0;
	tlPly.querySelector('.tl-total').innerHTML = tlPoints(playerWhite ? w : b);
	tlOpp.querySelector('.tl-total').innerHTML = tlPoints(playerWhite ? b : w);

	const plyGames = tlPly.querySelector('.tl-games');
	const oppGames = tlOpp.querySelector('.tl-games');
	plyGames.innerHTML = '';
	oppGames.innerHTML = '';

	// history entries are keyed by the players' CURRENT seats (the ScorePayload
	// convention); wp is the color the currently-white player held in that game
	(message.d.h || []).forEach((e, i) => {
		const myColor = playerWhite ? e.wp : (e.wp === 'w' ? 'b' : 'w');
		const oppColor = myColor === 'w' ? 'b' : 'w';
		plyGames.appendChild(tlCell(playerWhite ? e.w : e.b, myColor, e.r, i + 1));
		oppGames.appendChild(tlCell(playerWhite ? e.b : e.w, oppColor, e.r, i + 1));
	});

	// the in-progress game's column
	if (!gameOver) {
		const num = (message.d.h || []).length + 1;
		[plyGames, oppGames].forEach((rowEl) => {
			const cell = document.createElement('span');
			cell.className = 'tl-cell tl-live';
			cell.setAttribute('role', 'listitem');
			cell.title = `Game ${num}: in progress`;
			rowEl.appendChild(cell);
		});
	}

	// keep the newest games in view if the match outgrows the row
	plyGames.scrollLeft = plyGames.scrollWidth;
	oppGames.scrollLeft = oppGames.scrollWidth;
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

	// resolve which clock element is white vs black so the values and flash land
	// correctly. Uses the color cached from move messages (playerWhite), NOT
	// isPlayerWhite: game-over messages reuse the `w` key for the winner, so
	// isPlayerWhite misreads them and would swap the chips for a white player.
	const whiteScore = playerWhite ? plyScore : oppScore;
	const blackScore = playerWhite ? oppScore : plyScore;

	whiteScore.innerHTML = w;
	blackScore.innerHTML = b;

	// match score only changes at game end, so a positive delta is the trigger
	if (prevScoreW !== null) {
		flashScore(whiteScore, w - prevScoreW);
		flashScore(blackScore, b - prevScoreB);
	}
	prevScoreW = w;
	prevScoreB = b;

	// the same score-bearing messages drive the match timeline
	renderTimeline(message);
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

	const plyBar = plyClock.getElementsByClassName("clockProgressBar")[0];
	const oppBar = oppClock.getElementsByClassName("clockProgressBar")[0];

	if (casualGame) {
		// untimed casual game: static ∞ clocks — the active-turn and name-color
		// styling below still applies, but nothing counts down
		setClockInfinite(plyClock);
		setClockInfinite(oppClock);
	} else {
		// set clock times
		plyTime.innerHTML = timeFormatter(playerTimeRemaining);
		oppTime.innerHTML = timeFormatter(opponentTimeRemaining);

		// low-time emphasis (<10s = 1000 centiseconds): toggled on the clock wrapper
		// so the time + progress bar shift to the loss color and pulse (app.css .low)
		plyClock.classList.toggle('low', playerTimeRemaining < 1000);
		oppClock.classList.toggle('low', opponentTimeRemaining < 1000);

		// set time bar progress
		plyBar.style.width = barWidth(message.d.c.tc, playerTimeRemaining);
		oppBar.style.width = barWidth(message.d.c.tc, opponentTimeRemaining);
	}

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
	// the clock on regular game updates, which is not intended. Never once the
	// game is over: the clocks must stay frozen at their final values, and the
	// server re-answers board queries (reconnects, reconcile re-queries) with
	// the finished game's state, whose moves would otherwise re-arm the ticker
	// with no game-over message coming to cancel it (repeats return early).
	// Casual games never tick — the ∞ display is static.
	if (message.d.m && !gameOver && !casualGame) {
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

// the game's time control in centi-seconds, rendered into the board container
// server-side so it is available even before the first board message. It is the
// full clock both sides start every game with, so it drives the clock reset the
// blind deploy phase needs (see resetClocksToFull): the deploy-start message
// carries no clock, so without this the arrange phase would keep showing the
// previous game's final times until the reveal.
const timeControlCenti = parseInt(
	(document.getElementById('gcon-xx') || {}).dataset?.tc || '0', 10);

// casual (untimed) game: the server clock is effectively infinite, so the
// clocks render as a static ∞ — no ticker, no low-time emphasis, full bars.
const casualGame =
	(document.getElementById('gcon-xx') || {}).dataset?.casual === 'true';

/**
 * setClockInfinite renders one clock as the casual ∞ display: static glyph,
 * full progress bar, and no low-time emphasis on the wrapper.
 * @param clockEl - the #clockPlayer / #clockOpponent wrapper
 */
const setClockInfinite = (clockEl) => {
	if (!clockEl) {
		return;
	}
	const t = clockEl.getElementsByClassName('clockTime')[0];
	const bar = clockEl.getElementsByClassName('clockProgressBar')[0];
	if (t) { t.innerHTML = '&infin;'; }
	if (bar) { bar.style.width = '100%'; }
	clockEl.classList.remove('low');
};

/**
 * resetClocksToFull sets both clock displays back to the full time control —
 * the state every game (and the blind deploy phase before it) begins from. The
 * server has already swapped in a fresh full-time clock for the next game; this
 * reflects that on the client while no board message is flowing (the deploy
 * arrange phase). Cancels any running interpolator so a stale ticker can't fight
 * the reset, and clears the low-time / active emphasis.
 */
const resetClocksToFull = () => {
	if (!timeControlCenti) {
		return;
	}
	cancelAnimationFrame(frameId);
	if (casualGame) {
		// full time in a casual game is the same static ∞
		[clockPlayerEl, clockOpponentEl].forEach((clk) => {
			setClockInfinite(clk);
			if (clk) { clk.classList.remove('active'); }
		});
		return;
	}
	const full = timeFormatter(timeControlCenti);
	[clockPlayerEl, clockOpponentEl].forEach((clk) => {
		if (!clk) {
			return;
		}
		const t = clk.getElementsByClassName('clockTime')[0];
		const bar = clk.getElementsByClassName('clockProgressBar')[0];
		if (t) { t.innerHTML = full; }
		if (bar) { bar.style.width = '100%'; }
		clk.classList.remove('low', 'active');
	});
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
 * Disable board if disconnected. Also greys the seat presence indicators:
 * with the socket down nobody's presence is knowable (a reconnect's crowd
 * broadcast restores them).
 */
const disableBoard = () => {
	og.set({
		movable: {
			free: false,
			dests: new Map(),
		}
	});
	clearPresence();
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
	// only meaningful while the result overlay is showing (or about to show)
	// a live window
	if (!resultShowingOrPending()) {
		return;
	}

	// a spectator has no rematch controls or opponent-left state to manage;
	// just keep the window countdown honest and skip the rest
	if (isSpec) {
		if (message.d.s) {
			startCountdown(message.d.s, rematchWindowLabel);
		}
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

	// a lock update ({lk}) reports a side committed; update the indicator only.
	// Outside the phase it can only be a straggler from a phase this client has
	// already left — drop it, or the indicator would linger over the live board.
	if (d.lk) {
		if (deployMode || deploySpectating) {
			updateDeployLock(d.lk);
		}
		return;
	}

	// stale-phase guard: a deploy message names the pre-deploy game it
	// supersedes (d.i). If we're not in the phase, no game-over is showing, and
	// that id isn't the game we're tracking, the message describes a phase this
	// client has already moved past — e.g. a deploy-state response built
	// mid-phase but delivered after the reveal. Entering deploy mode over the
	// live game would wedge us there permanently (every later snapshot carries
	// the id of the game we'd be blind-covering, so nothing could ever read as
	// new). A legitimate phase entry always passes: a rematch phase follows a
	// processed game-over (gameOver true), and a page reload into a live phase
	// arrives with currentGameID still unset.
	if (!deployMode && !deploySpectating && !gameOver
		&& d.i && currentGameID && d.i !== currentGameID) {
		return;
	}

	const uid = getCookie('uid');
	const seconds = d.s ? d.s : 30;
	// derive our side from the message's player ids rather than the DOM
	// orientation class, which is stale after a rematch swaps colors. A spectator
	// matches neither id and watches the blind phase (both ranks hidden); the
	// explicit isSpec check is belt-and-braces for the same viewer.
	if (isSpec || (uid !== d.w && uid !== d.b)) {
		enterDeploySpectatorMode(d);
		return;
	}
	deployIsWhite = (uid === d.w);
	enterDeployMode(seconds, d);
};

const enterDeployMode = (seconds, payload) => {
	const d = payload || {};
	if (deployMode) {
		// already arranging (a reconnect's deploy-state response, or the
		// server's periodic re-announce): refresh lock indicators and reconcile
		// confirmation. If we believe we confirmed but the server has no
		// arrangement for us — no cf (unicast deploy-state) AND our own color not
		// locked (the broadcast announce carries no per-recipient cf, but its
		// lw/lb are truthful) — the submission was lost in transit, so make sure
		// the resend loop is running rather than sitting on "waiting" until the
		// server's deploy-timeout autofill while the opponent waits out the window.
		if (d.lw) { updateDeployLock('white'); }
		if (d.lb) { updateDeployLock('black'); }
		if (d.cf) { markDeploySubmitAcked(); }
		if (deployConfirmed && !deploySubmitAcked) {
			sendDeploySubmit();
		}
		return;
	}
	deployMode = true;
	deployConfirmed = false;
	deploySubmitAcked = false;
	clearTimeout(deploySubmitRetryTimer);
	deploySubmitRetryTimer = null;
	deployLockWhite = false;
	deployLockBlack = false;
	// baseline for reveal recognition: every id known to predate this phase.
	// d.i is the pre-deploy game the server says this phase supersedes (during
	// a rematch, the placeholder swapped in at agreement — an id this client
	// may never have seen); currentGameID is whatever we were tracking (during
	// a rematch, the finished game). Any board state carrying an id outside
	// this set is the deployed game (see handleMove's newGame test).
	deployPriorGameIDs = [d.i, currentGameID].filter(Boolean);

	// a deploy phase begins a new game, so clear any lingering game-over /
	// rematch overlay from the previous game
	hideResult();

	// the next game starts both clocks at full time; show that now rather than
	// leaving the previous game's final times up through the whole arrange phase
	// (the deploy-start message carries no clock — the reveal is the next one that
	// does)
	resetClocksToFull();
	// the previous game's material diff is stale for the same reason (the reveal
	// re-renders it; deploy positions themselves are hidden/partial by design)
	clearMaterial();

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
	// a server-known confirm (d.cf) means our arrangement is already recorded — ack
	// it so the resend loop confirmDeploy just started stops right away
	if (d.cf) { markDeploySubmitAcked(); }
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
	// same reveal-recognition baseline as the player path (see enterDeployMode)
	deployPriorGameIDs = [d.i, currentGameID].filter(Boolean);
	hideResult();

	// the next game starts both clocks at full time (see enterDeployMode); the
	// previous game's material diff is stale for the same reason
	resetClocksToFull();
	clearMaterial();

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

	// a spectator is also waiting on the single reveal broadcast, with no
	// confirm of their own to hang the poll off — start it on phase entry
	startDeployResync();
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

// deploySubmitRetryMs paces the resend loop below — fast enough to recover a lost
// confirm within a second, slow enough not to spam the socket.
const deploySubmitRetryMs = 1000;

/**
 * sendDeploySubmit transmits our committed arrangement and schedules a resend, so
 * a submit lost to a connectivity blip (a half-open socket, a dropped frame) is
 * retried until the server acknowledges it — the deploy analogue of a move's
 * pending/ack resend. The server acks by locking our color (the broadcast lock, an
 * announce's lw/lb, or the unicast deploy-state cf), which markDeploySubmitAcked
 * observes to stop the loop; exitDeployMode stops it when the phase ends. Retries
 * also cease once the deploy deadline has passed (the server has autofilled by
 * then), so a permanently dead socket can't spin the timer forever.
 *
 * The order is read from our authoritative model (readDeployOrderFromModel), not
 * the live og.state.pieces: onDeploySwap defers the board re-render by one
 * setTimeout(0) tick, during which the live board holds only three pieces (the
 * destination piece was overwritten by octadground's move). A submit landing in
 * that window would read a 3-char order the server rejects; the model is updated
 * synchronously in onDeploySwap, so it is always complete (what saveDeployDraft
 * already trusts).
 */
const sendDeploySubmit = () => {
	clearTimeout(deploySubmitRetryTimer);
	deploySubmitRetryTimer = null;
	if (!deployMode || !deployConfirmed || deploySubmitAcked) {
		return;
	}
	send(buildCommand(deployTag, { o: readDeployOrderFromModel() }));
	// past the deadline the arrangement is the server's autofill to make; there is
	// nothing to gain from resending, so let the loop end.
	if (deployDeadline && Date.now() > deployDeadline + 2000) {
		return;
	}
	deploySubmitRetryTimer = setTimeout(sendDeploySubmit, deploySubmitRetryMs);
};

/**
 * markDeploySubmitAcked records that the server confirmed receipt of our
 * arrangement (our color is locked, or a unicast deploy-state carried cf) and
 * stops the resend loop.
 */
const markDeploySubmitAcked = () => {
	if (deploySubmitAcked) {
		return;
	}
	deploySubmitAcked = true;
	clearTimeout(deploySubmitRetryTimer);
	deploySubmitRetryTimer = null;
};

const confirmDeploy = () => {
	if (deployConfirmed || !deployMode) {
		return;
	}
	deployConfirmed = true;
	deploySubmitAcked = false;
	// persist the confirmed state so a refresh re-enters locked, not unconfirmed
	saveDeployDraft(true);
	// submit the arrangement and keep resending until the server acks it
	sendDeploySubmit();
	// lock the board and switch the controls to the waiting state
	og.set({ draggable: { enabled: false }, selectable: { enabled: false }, movable: { free: false, color: undefined, dests: new Map() } });
	document.getElementById('deploy-confirm').classList.add('hidden');
	document.getElementById('deploy-waiting').classList.remove('hidden');
	renderDeployLock();
	// guard against missing the single reveal broadcast (and against this
	// confirm itself being lost in transit): poll for authoritative state until
	// the deployed game begins (see startDeployResync)
	startDeployResync();
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
	// our own color locking is the server's acknowledgement that it recorded our
	// submission — stop the resend loop
	if (deployConfirmed && color === (deployIsWhite ? 'white' : 'black')) {
		markDeploySubmitAcked();
	}
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
	deploySubmitAcked = false;
	clearTimeout(deploySubmitRetryTimer);
	deploySubmitRetryTimer = null;
	clearDeployCountdown();
	clearDeployDraft();
	stopDeployResync();

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
 * playerColor returns the viewer's board orientation color — the local
 * player's, or the anchored player's current color for a spectator.
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
	updateMaterial(parts[0]);
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
	// touching any navigation control while the result card is up dismisses it,
	// so the modal never blocks reviewing the finished game
	dismissResultForAnalysis();
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
		// scroll only the move-list panel ("nearest" semantics by hand):
		// scrollIntoView also scrolls the page itself, which on mobile yanks
		// the viewport down to the move list on every move.
		const listRect = moveListEl.getBoundingClientRect();
		const cellRect = active.getBoundingClientRect();
		if (cellRect.top < listRect.top) {
			moveListEl.scrollTop += cellRect.top - listRect.top;
		} else if (cellRect.bottom > listRect.bottom) {
			moveListEl.scrollTop += cellRect.bottom - listRect.bottom;
		}
	}
	updateNavButtons();
	// every ply change funnels through here (nav buttons, keys, move-list
	// clicks, live updates), so the last-move annotation stays in step
	updateEndAnnotation();
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

// ---- copy PGN (analysis mode) ----

// the standard octad starting position; a game that began anywhere else (a
// blind-deploy game) gets SetUp/FEN tags so the movetext replays correctly
const standardStartOFEN = 'ppkn/4/4/NKPP w NCFncf - 0 1';

/** Escape a PGN tag value (backslashes and double quotes). */
const pgnEscape = (s) => String(s).replace(/\\/g, '\\\\').replace(/"/g, '\\"');

/**
 * buildPGN assembles the finished game's PGN from the server-sent per-ply
 * history, mirroring the server's archival buildArchivePGN: a tag roster, a
 * SetUp/FEN pair when the game began off the standard start, and numbered SAN
 * movetext ending in the result token. Player names come from the match
 * timeline rows, mapped to colors via the bottom-is-white/player convention
 * that orients the whole page (see isPlayerWhite).
 * @returns {string} the PGN text
 */
const buildPGN = () => {
	const sans = history.sans || [];
	const startOFEN = (history.ofens && history.ofens[0]) || standardStartOFEN;

	const nameOf = (sel) => {
		const el = document.querySelector(sel);
		return el ? el.textContent.trim() : '';
	};
	const bottomName = nameOf('#tl-row-player .tl-name') || 'Anonymous';
	const topName = nameOf('#tl-row-opponent .tl-name') || 'Anonymous';
	const whiteName = playerWhite ? bottomName : topName;
	const blackName = playerWhite ? topName : bottomName;

	const now = new Date();
	const pad = (n) => String(n).padStart(2, '0');
	const tags = [
		['Event', 'Lioctad Casual Game'],
		['Site', window.location.origin],
		['Date', now.getFullYear() + '.' + pad(now.getMonth() + 1) + '.' + pad(now.getDate())],
		['White', whiteName],
		['Black', blackName],
		['Result', gameResult],
	];
	const copyBtn = document.getElementById('btn-copy-pgn');
	if (copyBtn && copyBtn.dataset.variant) {
		tags.splice(2, 0, ['Variant', copyBtn.dataset.variant]);
	}
	if (startOFEN !== standardStartOFEN) {
		tags.push(['SetUp', '1'], ['FEN', startOFEN]);
	}

	// movetext numbering seeded from the start position's side-to-move and
	// fullmove fields, so deploy starts number correctly too
	const fields = startOFEN.split(' ');
	let moveNum = parseInt(fields[5], 10) || 1;
	let whiteToPlay = fields[1] !== 'b';
	const tokens = [];
	sans.forEach((san, i) => {
		if (whiteToPlay) {
			tokens.push(moveNum + '. ' + san);
		} else {
			tokens.push(i === 0 ? moveNum + '... ' + san : san);
			moveNum++;
		}
		whiteToPlay = !whiteToPlay;
	});
	tokens.push(gameResult);

	return tags.map(([k, v]) => '[' + k + ' "' + pgnEscape(v) + '"]').join('\n')
		+ '\n\n' + tokens.join(' ') + '\n';
};

// clipboard write with a hidden-textarea fallback for insecure contexts (e.g.
// LAN-IP dev on a phone, where navigator.clipboard is unavailable)
const copyTextToClipboard = (text, done) => {
	const fallback = () => {
		const ta = document.createElement('textarea');
		ta.value = text;
		ta.setAttribute('readonly', '');
		ta.style.position = 'fixed';
		ta.style.opacity = '0';
		document.body.appendChild(ta);
		ta.select();
		try {
			if (document.execCommand('copy')) {
				done();
			}
		} finally {
			document.body.removeChild(ta);
		}
	};
	if (navigator.clipboard && navigator.clipboard.writeText) {
		navigator.clipboard.writeText(text).then(done).catch(fallback);
	} else {
		fallback();
	}
};

const copyPgnBtn = document.getElementById('btn-copy-pgn');
let copyPgnTimer = null;
if (copyPgnBtn) {
	copyPgnBtn.addEventListener('click', () => {
		copyTextToClipboard(buildPGN(), () => {
			// flash the check icon as copy confirmation
			copyPgnBtn.classList.add('copied');
			clearTimeout(copyPgnTimer);
			copyPgnTimer = setTimeout(() => copyPgnBtn.classList.remove('copied'), 1500);
		});
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

// crowd messages: the room view layers seat presence on top of the base
// spectator-count handler in lio.js (lio.js loads first, so setting the same
// tag replaces it).
window.handlers.set(crowdTag, (message) => {
	const crowdEl = document.getElementById("crowd");
	if (crowdEl) {
		crowdEl.innerHTML = message.d.s;
	}
	updatePresence(!!message.d.w, !!message.d.b);
});

/**
 * handleIdentity processes the server's one-shot socket identity echo: the uid
 * this connection authenticated as and whether it was seated as a spectator
 * (seat membership is fixed at upgrade time). A page rendered for a seated
 * player whose socket lands as a spectator is an identity desync — iOS Safari
 * intermittently omits the identity cookies from WS upgrade requests — and
 * every game frame this socket sends (moves, deploy submissions, rematch
 * clicks) would be silently dropped server-side while broadcasts still arrive,
 * making the game look almost-alive. Re-authenticate with one guarded reload
 * (a full navigation reliably carries/re-mints the cookies); a consistent echo
 * re-arms the one-shot recovery. Overrides the base handler in lio.js.
 */
const handleIdentity = (message) => {
	const seatedSpectator = !!(message.d && message.d.s);
	if (!isSpec && seatedSpectator) {
		forceIdentityReload('player page on spectator socket');
		return;
	}
	// identity is consistent with the page; re-arm the one-shot reload recovery
	try { sessionStorage.removeItem(identityReloadKey); } catch (e) { /* noop */ }
};

window.handlers.set(moveTag, handleMove);
window.handlers.set(gameOverTag, handleGameOver);
window.handlers.set(rematchUpdateTag, handleRematchUpdate);
window.handlers.set(drawOfferTag, handleDrawOffer);
window.handlers.set(deployTag, handleDeploy);
window.handlers.set("id", handleIdentity);
