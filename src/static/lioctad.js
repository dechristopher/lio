let ws, ka, backoff = 0, move = 1;

const logMe = () => console.log(`© 2021 lioctad.org`);

const moveSound = new Audio("/res/sfx/move.ogg");
moveSound.volume = 1;
const capSound = new Audio("/res/sfx/capture.ogg");
capSound.volume = 1;
const endSound = new Audio("/res/sfx/end.ogg");
endSound.volume = 0.75;

// create game board
let og = Octadground(document.getElementById('game'), {
	highlight: {
		lastMove: true,
		check: true,
	},
	movable: {
		free: false,
		color: 'white',
		events: {
			after: (orig, dest, meta) => {
				if (meta.captured) {
					capSound.play();
				} else {
					moveSound.play();
				}
			}
		}
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
		/^http/, 'ws')}/ws/test`);

	ws.onopen = () => {
		console.log("Connected to lioctad.org");
		connected();
	};

	ws.onclose = () => {
		console.warn("Lost connection to lioctad.org");
		ws = null;
		clearInterval(ka);
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
 * Increment the backoff time so we don't flood the backend
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
 * Send a JSON stringified command over the websocket
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

	switch (message.t) {
		case "m": // move happened
			if (!message.d.m) {
				move = 1;
				document.getElementById("info").innerHTML = ""
					+ "FREE, ONLINE OCTAD COMING SOON!";
			}
			const ofenParts = message.d.o.split(' ');
			og.set({
				ofen: ofenParts[0],
				lastMove: getLastMove(message.d.m),
				turnColor: ofenParts[1] === "w" ? "white" : "black",
				check: message.d.k,
				movable: {
					free: false,
					dests: allMoves(message.d.v),
				}
			});
			if (message.d.s) {
				playSound(message.d.s)
			}
			// perform pre-move if set
			og.playPremove();
			break;
		case "g": // game over
			document.getElementById("info").innerHTML = message.d.s;
			endSound.play();
			break;
		default:
			return;
	}
};

/**
 * Return a map of all legal moves
 * @param moves - raw moves object
 * @returns {Map<string, string>}
 */
const allMoves = (moves) => {
	let allMoves = new Map();
	if (!!moves) {
		Object.entries(moves).forEach(([s1, dests]) => {
			allMoves.set(s1, dests);
		});
	}
	return allMoves;
}

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
}

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
const doMove= (orig, dest) => {
	let promo = "";
	if (og.state.pieces.get(dest) && og.state.pieces.get(dest).role === "pawn") {
		let destPiece = og.state.pieces.get(dest);
		// TODO prompt for promo piece type
		if (destPiece.color === "white" && dest[1] === "4") {
			promo = 'q';
		} else if (destPiece.color === "white" && dest[1] === "1") {
			promo = 'q';
		}
	}

	sendGameMove(orig + dest + promo, move);
	move++;
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
}