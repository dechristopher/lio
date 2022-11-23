// LIO game handling code
let frameId, frameTime, wt, bt, move = 1;

const moveTag = "m";
const gameOverTag = "g";
const rematchTag = "re";

const StatusWhiteWinsCheckmate = 2;
const StatusBlackWinsCheckmate = 3;
const StatusWhiteWinsResignation = 4;
const StatusBlackWinsResignation = 5;
const StatusWhiteWinsTimeout = 6;
const StatusBlackWinsTimeout = 7;
const StatusDrawAgreement = 8;
const StatusDrawInsufficientMaterial = 9;
const StatusDrawRepetition = 10;
const StatusDrawStalemate = 11;
const StatusDrawTwentyFiveMoveRule = 12;

const CheckmateMessage = "by checkmate";
const ResignationMessage = "opponent resigned";
const TimeoutMessage = "opponent resigned";
const DrawAgreementMessage = "by agreement";
const DrawInsufficientMaterialMessage = "insufficient material";
const DrawRepetitionMessage = "via repetition";
const DrawStalemateMessage = "via stalemate";
const DrawTwentyFiveMoveRuleMessage = "25-move rule exceeded"

const GameOverReasonMap = {
	[StatusWhiteWinsCheckmate]: CheckmateMessage,
	[StatusBlackWinsCheckmate]: CheckmateMessage,
	[StatusWhiteWinsResignation]: ResignationMessage,
	[StatusBlackWinsResignation]: ResignationMessage,
	[StatusWhiteWinsTimeout]: TimeoutMessage,
	[StatusBlackWinsTimeout]: TimeoutMessage,
	[StatusDrawAgreement]: DrawAgreementMessage,
	[StatusDrawInsufficientMaterial]: DrawInsufficientMaterialMessage,
	[StatusDrawRepetition]: DrawRepetitionMessage,
	[StatusDrawStalemate]: DrawStalemateMessage,
	[StatusDrawTwentyFiveMoveRule]: DrawTwentyFiveMoveRuleMessage,
};

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
 * Sends a game move in Universal Octad Interface format
 * @param move - UOI move string
 * @param num - move number
 */
const sendGameMove = (move, num) => {
	send(buildCommand(moveTag, {
		u: move,
		a: num
	}));
};

/**
 * Sends a game rematch request
 */
const sendRematchRequest = () => {
	send(buildCommand(rematchTag, {r: true}));
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
 * @param message - move message
 * @param ofenParts - split OFEN
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

/**
 * Handle incoming move messages, update board state, update UI and clocks
 * @param message - move message
 */
const handleMove = (message) => {
	if (!message.d.m) {
		move = 1;
		hideRematchModal();
	}

	const ofenParts = message.d.o.split(' ');

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

	// perform pre-move if set
	og.playPremove();
};

/**
 * Handles incoming game over messages
 * @param message - game over message
 */
const handleGameOver = (message) => {
	// if room over, redirect home after a second
	if (message.d.o === true) {
		disconnect("room over");
		// TODO show room over message on modal if still up, or on game interface?
		// setTimeout(() => {
		//  // redirect home
		// 	window.location.href = "/";
		// }, 3000);
		return;
	}

	cancelAnimationFrame(frameId);
	window.notification.play();

	// disallow further moves
	og.set({
		movable: {
			dests: new Map()
		}
	});

	// update match score
	updateScore(message);

	// display the rematch modal
	displayRematch(message);
};

/**
 * Handles incoming rematch information messages
 * @param message - rematch information message
 */
const handleRematch = (message) => {
	// TODO
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

	if (message.d.sc.w) {
		// move message, with scores keyed by color
		if (isPlayerWhite(message)) {
			plyScore.innerHTML = message.d.sc.w;
			oppScore.innerHTML = message.d.sc.b;
		} else {
			plyScore.innerHTML = message.d.sc.b;
			oppScore.innerHTML = message.d.sc.w;
		}
	} else {
		// game over message, with scores keyed by player uid
		Object.keys(message.d.sc).forEach((uid) => {
			if (uid === getCookie("uid")) {
				plyScore.innerHTML = message.d.sc[uid];
			} else {
				oppScore.innerHTML = message.d.sc[uid];
			}
		});
	}
};

/**
 * Displays the updated rematch modal using the given message
 * @param message - game over message
 */
const displayRematch = (message) => {
	// update win/loss message
	updateRematchModalWinLoss(message);

	// update win/loss reason message
	updateRematchModalReason(message);

	// update winner picture border
	updateRematchWinnerPictureBorder(message);

	// update scores
	updateRematchModalScores(message);

	// set rematch button handler
	document.getElementById("rematch")
		.addEventListener('click', sendRematch);

	// display modal after 3/4 of a second
	setTimeout(() => {
		openRematchModal();
	}, 750);
};

/**
 * Sends a rematch message request
 */
const sendRematch = () => {
	sendRematchRequest();
	const rematchButton = document.getElementById("rematch");
	// style rematch button green after rematch request sent
	rematchButton.classList.add("sent");
	// remove rematch button click event
	rematchButton.removeEventListener("click", sendRematch);
};

/**
 * Updates the rematch modal's win/loss message
 * @param message - game over message
 */
const updateRematchModalWinLoss = (message) => {
	const winLoss = document.getElementById("win-loss");

	if (message.d.wc === "d") {
		winLoss.innerHTML = "Game drawn"
		return;
	}

	const outcome = document.getElementById("outcome");

	if (isPlayerWinner(message)) {
		winLoss.innerHTML = "You won!";
		outcome.classList.add("win");

	} else {
		winLoss.innerHTML = "You lost";
		outcome.classList.add("loss");
	}
};

/**
 * Updates the rematch modals' win/loss reason message
 * @param message - game over message
 */
const updateRematchModalReason = (message) => {
	const reason = document.getElementById("reason");
	if (message.d.sid) {
		reason.innerHTML = GameOverReasonMap[message.d.sid];
	} else {
		reason.innerHTML = "via an unknown reason";
	}
};

/**
 * Updates the profile picture border of the winning player
 * @param message - game over message
 */
const updateRematchWinnerPictureBorder = (message) => {
	// no profile picture borders for a draw
	if (message.d.wc === "d") {
		return;
	}

	let picture;

	if (isPlayerWinner(message)) {
		picture = document.getElementById("player-icon");
	} else {
		picture = document.getElementById("opponent-icon");
	}

	picture.classList.add("winner");
};

/**
 * Updates the rematch modal's scores
 * @param message - game over message
 */
const updateRematchModalScores = (message) => {
	Object.keys(message.d.sc).forEach((uid) => {
		if (uid === getCookie("uid")) {
			document.getElementById("score-player").innerHTML = message.d.sc[uid];
		} else {
			document.getElementById("score-opponent").innerHTML = message.d.sc[uid];
		}
	});
};

/**
 * Returns true if the player is the winner of the game
 * @param message - game over message
 * @returns {boolean} player is winner of game
 */
const isPlayerWinner = (message) => {
	return message.d.wid === getCookie('uid');
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
window.handlers.set(rematchTag, handleRematch);
