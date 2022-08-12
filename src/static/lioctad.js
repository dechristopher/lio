let ws, ka, backoff = 0, move = 1;
let pingRunner, lastPingTime, latency = 0, pongCount = 0, pingDelay = 3000;
let clockTicker, wt, bt;

const logMe = () => console.log(`© 2022 lioctad.org`);

const moveSound = new Howl({
	src: ["/res/sfx/move.ogg"],
	preload: true,
	autoplay: true,
	html5: true,
	volume: 0.9
});

const capSound = new Howl({
	src: ["/res/sfx/capture.ogg"],
	preload: true,
	volume: 0.9
});

const confirmation = new Howl({
	src: ["/res/sfx/confirmation.ogg"],
	preload: true,
	volume: 0.99
});

const notification = new Howl({
	src: ["/res/sfx/end.ogg"],
	preload: true,
	volume: 0.6
});

// create game board
let og = Octadground(document.getElementById('game'), {
	orientation: document.getElementById('gcon-xx').classList.contains('white') ? 'white' : 'black',
	highlight: {
		lastMove: true,
		check: true,
	},
	movable: {
		free: false,
		color: document.getElementById('gcon-xx').classList.contains('white') ? 'white' : 'black'
	},
	selectable: {
		enabled: false,
	},
	events: {
		move: (orig, dest, capturedPiece) => {
			doMove(orig, dest);
		}
	}
});

// connect on page load
window.addEventListener('load', () => {
	logMe();

	if (ws) {
		return false;
	}
	connect();
	return true;
});

/**
 * Connect to the backend and handle events
 */
const connect = () => {
	ws = new WebSocket(`${location.origin.replace(
		/^http/, 'ws')}/ws${location.pathname}`);

	ws.onopen = () => {
		console.log("Connected to lioctad.org");
		connected();
	};

	ws.onclose = () => {
		console.warn("Lost connection to lioctad.org");
		ws = null;
		clearInterval(ka);
		clearInterval(pingRunner);
		disableBoard();
		reconnect();
	};

	ws.onmessage = (evt) => {
		parseResponse(evt.data);
	};
};

/**
 * We've connected! Enable stuff!
 */
const connected = () => {
	backoff = 0;
	sendBoardUpdateRequest();
	schedulePing(500);
	ka = setInterval(() => {
		sendKeepAlive();
	}, 3000);
};

/**
 * Reconnect to the backend adhering to exponential backoff
 */
const reconnect = () => {
	incrBackoff();
	setTimeout(() => {
		connect();
	}, backoff * 1000);
};

/**
 * Increment the backoff time so that we don't flood the backend
 */
const incrBackoff = () => {
	if (backoff === 0) {
		backoff = 1;
	} else if (backoff <= 4) {
		backoff *= 2;
	}
	console.log("Waiting " + backoff + " seconds to retry...");
};

/**
 * Send a JSON string command over the websocket
 * @param command - command object
 */
const send = (command) => {
	if (ws && ws.readyState === WebSocket.OPEN) {
		ws.send(command);
	}
};

/**
 * Sends a keep-alive message, requesting the socket stay open
 */
const sendKeepAlive = () => {
	send(null);
};

/**
 * Sends an empty move message to prompt a response with board info
 */
const sendBoardUpdateRequest = () => {
	send(buildCommand("m", {a: 0}));
};

/**
 * Schedule a ping message after the specified delay
 * @param delay - delay in ms to wait before pinging
 */
const schedulePing = (delay) => {
	clearTimeout(pingRunner);
	pingRunner = setTimeout(ping, delay)
};

/**
 * Send a ping immediately
 */
const ping = () => {
	try {
		send(JSON.stringify({"pi": 1}));
		lastPingTime = Date.now();
	} catch (e) {
		console.debug(e, true);
	}
};

/**
 * Handle pong response, calculating latency
 */
const pong = () => {
	schedulePing(pingDelay);
	const currentLag = Math.min(Date.now() - lastPingTime, 10000);
	pongCount++;

	// average first few pings and then move to weighted moving average
	const weight = pongCount > 4 ? 0.1 : 1 / pongCount;
	latency += weight * (currentLag - latency);
	document.getElementById("lat").innerHTML = latency.toFixed(1);
};

/**
 * Sends a game move in Universal Octad Interface format
 * @param move - UOI move string
 * @param num - move number
 */
const sendGameMove = (move, num) => {
	send(buildCommand("m", {
		u: move,
		a: num
	}));
};

/**
 * Build socket message
 * @param tag - message tag
 * @param data - message payload data
 */
const buildCommand = (tag, data) => {
	let m = {
		t: tag,
		d: data,
	}
	return JSON.stringify(m);
};

/**
 * Determine what to do with received responses
 * @param raw - the raw message JSON string
 */
const parseResponse = (raw) => {
	if (!raw) {
		return
	}

	let message = JSON.parse(raw);

	// handle pongs
	if (message.po && message.po === 1) {
		pong();
		return;
	}

	switch (message.t) {
		case "m": // move happened
			handleMove(message);
			break;
		case "g": // game over
			clearInterval(clockTicker);
			document.getElementById("info").innerHTML = message.d.s;
			notification.play();

			// if room over, redirect home after a second
			if (message.d.o === true) {
				setTimeout(() => {
					window.location.href = "/";
				}, 3000);
			}
			break;
		case "c":
			document.getElementById("crowd").innerHTML = message.d.s;
			break;
		default:
			return;
	}
};

/**
 * Returns true if the player is playing white
 * @param message - move message
 * @returns {boolean} is white
 */
const isPlayerWhite = (message) => {
	return message.d.w === getCookie('bid');
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

/**
 * Handle incoming move messages, update board state, update UI and clocks
 * @param message
 */
const handleMove = (message) => {
	if (!message.d.m) {
		move = 1;
		document.getElementById("info").innerHTML = "";
	}

	const ofenParts = message.d.o.split(' ');
	og.set({
		orientation: message.d.w === getCookie('bid') ? 'white' : 'black',
		ofen: ofenParts[0],
		lastMove: getLastMove(message.d.m),
		turnColor: whiteToMove(ofenParts) ? "white" : "black",
		check: message.d.k,
		movable: {
			free: false,
			dests: allMoves(message.d.v),
			color: message.d.w === getCookie('bid') ? 'white' : 'black',
		}
	});

	if (message.d.gs) {
		confirmation.play();
	}

	if (message.d.s) {
		playSound(message.d.s);
	}

	// update UI styles and clock tickers
	updateUI(message, ofenParts);

	// perform pre-move if set
	og.playPremove();
};

/**
 * updateUI updates UI state, styles and clock tickers
 * @param message - move message
 * @param ofenParts - OFEN parts array
 */
const updateUI = (message, ofenParts) => {
	clearInterval(clockTicker);

	wt = message.d.c.w;
	bt = message.d.c.b;

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
		oppClock.classList.remove('playerWhite');
		plyClock.classList.add('playerBlack');
		plyClock.classList.remove('playerWhite');
	}

	// only run this when move is provided, otherwise we flip
	// the clock on regular game updates, which is not intended
	if (message.d.m) {
		// reset centi-second clock interpolator to decrement correct player
		if (isPlayerTurn(message, ofenParts)) {
			clockTicker = setInterval(() => {
				if (playerTimeRemaining <= 10) {
					playerTimeRemaining = 0;
				} else {
					playerTimeRemaining -= 10;
				}
				plyTime.innerHTML = timeFormatter(playerTimeRemaining);
				plyBar.style.width = barWidth(message.d.c.tc, playerTimeRemaining);
			}, 100);
		} else {
			clockTicker = setInterval(() => {
				if (opponentTimeRemaining <= 10) {
					opponentTimeRemaining = 0;
				} else {
					opponentTimeRemaining -= 10;
				}
				oppTime.innerHTML = timeFormatter(opponentTimeRemaining);
				oppBar.style.width = barWidth(message.d.c.tc, opponentTimeRemaining);
			}, 100);
		}
	}
};

const padZero = (time) => `0${time}`.slice(-2);

/**
 * Format time in MM:SS.CC
 * @param centiseconds - number of centi-seconds remaining
 * @returns {string} formatted time
 */
const timeFormatter = (centiseconds) => {
	const minutes = centiseconds / 6000 | 0;
	const seconds = padZero((centiseconds / 100 | 0) % 60);
	const centi = padZero(`${centiseconds % 100}`.slice(-3));

	if (seconds < 10) {
		return `${minutes}:${seconds}.${centi}`;
	} else {
		return `${minutes}:${seconds}`;
	}
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
			document.getElementById("promo-shade-xx").classList.remove('hidden');
			document.getElementById("promo-xx").classList.remove('hidden');

			// set file for promo bar
			document.getElementById("promo-xx").classList.add(`f${dest[0]}`);

			// set piece selector colors and event handlers
			let promoButtons = document.getElementsByClassName('promo-piece');
			for(let i = 0; i < promoButtons.length; i++) {
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
	document.getElementById("promo-shade-xx").classList.add('hidden');
	document.getElementById("promo-xx").classList.add('hidden');

	// unset file for promo bar
	document.getElementById("promo-xx").classList.remove(`f${dest[0]}`);

	// unset promo piece color
	let promoButtons = document.getElementsByClassName('promo-piece');
	for(let i = 0; i < promoButtons.length; i++) {
		promoButtons[i].classList.remove('white');
		promoButtons[i].classList.remove('black');
	}
}

/**
 * Play sounds for incoming moves based on the SAN for the move
 * @param san
 */
const playSound = (san) => {
	if (san.includes("x")) {
		capSound.play();
	} else {
		moveSound.play();
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
	for(let i = 0; i < ca.length; i++) {
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
