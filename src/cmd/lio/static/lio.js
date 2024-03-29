// LIO core client code
let ka, backoff = 0;
let disconnected = false;
let pingRunner, lastPingTime, latency = 0, pongCount = 0, pingDelay = 5000;

// ws handlers map
window.handlers = new Map();

window.confirmation = new Howl({
	src: ["/res/sfx/confirmation.ogg"],
	preload: true,
	volume: 0.99
});

window.notification = new Howl({
	src: ["/res/sfx/end.ogg"],
	preload: true,
	volume: 0.8
});

const crowdTag = "c";

const logMe = () => console.log(`© 2024 lioctad.org`);

// requestAnimFrame polyfill
window.requestAnimFrame = (function () {
	return window.requestAnimationFrame ||
		window.webkitRequestAnimationFrame ||
		window.mozRequestAnimationFrame ||
		function (callback) {
			window.setTimeout(callback, 1000 / 60);
		};
})();

window.isMobile = (function (agent) {
	return /(android|bb\d+|meego).+mobile|avantgo|bada\/|blackberry|blazer|compal|elaine|fennec|hiptop|iemobile|ip(hone|od)|iris|kindle|lge |maemo|midp|mmp|mobile.+firefox|netfront|opera m(ob|in)i|palm( os)?|phone|p(ixi|re)\/|plucker|pocket|psp|series(4|6)0|symbian|treo|up\.(browser|link)|vodafone|wap|windows ce|xda|xiino/i.test(agent) || /1207|6310|6590|3gso|4thp|50[1-6]i|770s|802s|a wa|abac|ac(er|oo|s\-)|ai(ko|rn)|al(av|ca|co)|amoi|an(ex|ny|yw)|aptu|ar(ch|go)|as(te|us)|attw|au(di|\-m|r |s )|avan|be(ck|ll|nq)|bi(lb|rd)|bl(ac|az)|br(e|v)w|bumb|bw\-(n|u)|c55\/|capi|ccwa|cdm\-|cell|chtm|cldc|cmd\-|co(mp|nd)|craw|da(it|ll|ng)|dbte|dc\-s|devi|dica|dmob|do(c|p)o|ds(12|\-d)|el(49|ai)|em(l2|ul)|er(ic|k0)|esl8|ez([4-7]0|os|wa|ze)|fetc|fly(\-|_)|g1 u|g560|gene|gf\-5|g\-mo|go(\.w|od)|gr(ad|un)|haie|hcit|hd\-(m|p|t)|hei\-|hi(pt|ta)|hp( i|ip)|hs\-c|ht(c(\-| |_|a|g|p|s|t)|tp)|hu(aw|tc)|i\-(20|go|ma)|i230|iac( |\-|\/)|ibro|idea|ig01|ikom|im1k|inno|ipaq|iris|ja(t|v)a|jbro|jemu|jigs|kddi|keji|kgt( |\/)|klon|kpt |kwc\-|kyo(c|k)|le(no|xi)|lg( g|\/(k|l|u)|50|54|\-[a-w])|libw|lynx|m1\-w|m3ga|m50\/|ma(te|ui|xo)|mc(01|21|ca)|m\-cr|me(rc|ri)|mi(o8|oa|ts)|mmef|mo(01|02|bi|de|do|t(\-| |o|v)|zz)|mt(50|p1|v )|mwbp|mywa|n10[0-2]|n20[2-3]|n30(0|2)|n50(0|2|5)|n7(0(0|1)|10)|ne((c|m)\-|on|tf|wf|wg|wt)|nok(6|i)|nzph|o2im|op(ti|wv)|oran|owg1|p800|pan(a|d|t)|pdxg|pg(13|\-([1-8]|c))|phil|pire|pl(ay|uc)|pn\-2|po(ck|rt|se)|prox|psio|pt\-g|qa\-a|qc(07|12|21|32|60|\-[2-7]|i\-)|qtek|r380|r600|raks|rim9|ro(ve|zo)|s55\/|sa(ge|ma|mm|ms|ny|va)|sc(01|h\-|oo|p\-)|sdk\/|se(c(\-|0|1)|47|mc|nd|ri)|sgh\-|shar|sie(\-|m)|sk\-0|sl(45|id)|sm(al|ar|b3|it|t5)|so(ft|ny)|sp(01|h\-|v\-|v )|sy(01|mb)|t2(18|50)|t6(00|10|18)|ta(gt|lk)|tcl\-|tdg\-|tel(i|m)|tim\-|t\-mo|to(pl|sh)|ts(70|m\-|m3|m5)|tx\-9|up(\.b|g1|si)|utst|v400|v750|veri|vi(rg|te)|vk(40|5[0-3]|\-v)|vm40|voda|vulc|vx(52|53|60|61|70|80|81|83|85|98)|w3c(\-| )|webc|whit|wi(g |nc|nw)|wmlb|wonu|x700|yas\-|your|zeto|zte\-/i.test(agent.substr(0, 4));
})(navigator.userAgent || navigator.vendor || window.opera);

// connect on page load
window.addEventListener('load', () => {
	logMe();
});

/**
 * Connect to the backend and handle events
 */
const connect = (prefix) => {
	window.ws = new WebSocket(`${location.origin.replace(
		/^http/, 'ws')}/socket${prefix ? `/${prefix}` : ''}${location.pathname}`);

	window.ws.onopen = () => {
		console.log("Connected to lioctad.org");
		connected();
	};

	window.ws.onclose = () => {
		window.ws = null;
		clearInterval(ka);
		clearInterval(pingRunner);

		if (!disconnected) {
			console.warn("Lost connection to lioctad.org");
			if (typeof disableBoard !== 'undefined') {
				disableBoard();
			}
			reconnect(prefix);
		} else {
			console.log("Disconnected from lioctad.org");
		}
	};

	window.ws.onmessage = (evt) => {
		parseResponse(evt.data);
	};
};

/**
 * We've connected! Enable stuff!
 */
const connected = () => {
	backoff = 0;
	if (typeof og !== 'undefined') {
		sendBoardUpdateRequest();
	}
	schedulePing(500);
	ka = setInterval(() => {
		sendKeepAlive();
	}, 5000);
};

/**
 * Disconnect from the websocket endpoint with the given reason
 * @param reason - human-readable ws close message
 */
const disconnect = (reason) => {
	disconnected = true;
	window.ws.close(1000, reason);
}

/**
 * Reconnect to the backend adhering to exponential backoff
 */
const reconnect = (prefix) => {
	incrBackoff();
	setTimeout(() => {
		connect(prefix);
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
	if (window.ws && window.ws.readyState === WebSocket.OPEN) {
		window.ws.send(command);
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
	const latencyDisplay = document.getElementById("lat");
	if (latencyDisplay) {
		latencyDisplay.innerHTML = `${Math.round(latency)}`;
	}
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

	// run handler for message by tag if one exists
	if (window.handlers.get(message.t)) {
		window.handlers.get(message.t)(message);
	}
};

/**
 * Handles incoming crowd messages
 * @param message - crowd message
 */
const handleCrowd = (message) => {
	document.getElementById("crowd").innerHTML = message.d.s;
};

// Set handlers
window.handlers.set(crowdTag, handleCrowd);
