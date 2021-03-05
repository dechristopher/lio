const logMe = () => console.log(`© 2021 lioctad.org`);

const moveSound = new Audio("/res/sfx/move.ogg");
moveSound.volume = 1;
const capSound = new Audio("/res/sfx/capture.ogg");
capSound.volume = 1;

// create game board
let og = Octadground(document.getElementById('game'), {
	highlight: {
		lastMove: true,
		check: true,
	},
	movable: {
		free: false,
		color: 'white',
	},
	selectable: {
		enabled: false,
	},
	events: {
		move: (orig, dest, capturedPiece) => {
			if (capturedPiece) {
				capSound.play();
			} else {
				moveSound.play();
			}

			let promo = "";
			if (og.state.pieces.get(dest) && og.state.pieces.get(dest).role === "pawn") {
				let destPiece = og.state.pieces.get(dest);
				if (destPiece.color === "white" && dest[1] === "4") {
					promo = 'q';
				} else if (destPiece.color === "white" && dest[1] === "1") {
					promo = 'q';
				}
			}

			sendGameMove(orig + dest + promo);
		}
	}
});

let ws, ka;
let backoff = 0;

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
	sendHello();
	sendGameUpdateRequest();
	setInterval(() => {
		sendKeepAlive();
	}, 30000)
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
	} else if (backoff <= 8) {
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
		ws.send(JSON.stringify(command));
	}
};

/**
 * Sends a keep-alive message, requesting the socket stay open
 */
const sendKeepAlive = () => {
	send(buildCommand(0, ["alive"]))
};

/**
 * Sends a hello message, requesting up to date information
 */
const sendHello = () => {
	send(buildCommand(1, []))
};

/**
 * Sends a game connect message, requesting up to date game state
 */
const sendGameUpdateRequest = () => {
	send(buildCommand(2, ["0"]))
};

/**
 * Sends a game move in Universal Octad Interface format
 * @param move - UOI move string
 */
const sendGameMove = (move) => {
	send(buildCommand(2, ["1", move]))
};

/**
 * Build command struct
 * @param commandType - command type
 * @param params - array of command parameters and components
 */
const buildCommand = (commandType, params) => {
	let command = {};

	command.t = Date.now();
	command.c = commandType;
	command.b = params;

	return command;
};

/**
 * Determine what to do with received responses
 * @param raw - the raw message JSON string
 */
const parseResponse = (raw) => {
	let message = JSON.parse(raw);

	// console.log(message);

	switch (message.c) {
		case -1:
			// Error handle!
			break;
		case 0:
			break;
		case 1:
			og.set({
				movable: {
					free: true
				}
			})
			break;
		case 2: // game update
			if (message.b[0] === "0") {
				og.set({
					ofen: message.b[1],
					lastMove: getLastMove(JSON.parse(message.b[3])),
					turnColor: message.b[2] === "w" ? "white" : "black",
					check: message.b[5] === "1",
					movable: {
						free: false,
						dests: allMoves(JSON.parse(message.b[4]))
					}
				});
			} else {
				og.set({
					ofen: message.b[1],
					lastMove: getLastMove(JSON.parse(message.b[3])),
					turnColor: message.b[2] === "w" ? "white" : "black",
					check: message.b[5] === "1",
					movable: {
						free: false
					}
				});
				document.getElementById("info").innerHTML = "GAME OVER!";
				setTimeout(() => {
					location.reload();
				}, 2000)
			}
			break;
		default:
			return;
	}

	// console.log('state', og.state);
};

/**
 * Return a map of all legal moves
 * @param moves - raw moves object
 * @returns {Map<string, string>}
 */
const allMoves = (moves) => {
	let allMoves = new Map();
	Object.entries(moves).forEach(([s1, dests]) => {
		allMoves.set(s1, dests);
	});
	return allMoves;
}

/**
 * Return the most recent game move for last move highlighting
 * @param moves - ordered list of all moves
 * @returns {[string, string]|*[]}
 */
const getLastMove = (moves) => {
	if (moves && moves.length > 0) {
		const move = moves[moves.length - 1]
		return [
			move.substring(0, 2),
			move.substring(2, 4)
		]
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
	})
};