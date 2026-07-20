const redirectTag = "e";
const roomUpdateTag = "r";

window.addEventListener('load', () => {
	// collapse the QR fold on phones so the invite link leads; it renders
	// open so the QR stays visible without JS and on desktop (where the
	// toggle is hidden). 34rem matches the .wait-hero CSS breakpoint.
	const qrFold = document.getElementById('qrFold');
	if (qrFold && !window.matchMedia('(min-width: 34rem)').matches) {
		qrFold.removeAttribute('open');
	}

	document.getElementById('copyInviteButton').onclick = () => {
		document.getElementById('copyInviteButton').classList.add('copied');
		navigator.clipboard.writeText(document.getElementById('gameInviteLink').value);
	};

	if (window.ws) {
		return false;
	}

	// connect to waiting room
	connect("wait");

	// in case we miss the redirect broadcast, request
	// updates from the room to redirect us to the game
	// when it is ready
	setInterval(() => {
		requestRoomUpdate();
	}, 5000);

	// listen for redirect messages (game-ready → the game URL, or a gone room →
	// its archive permalink / home). Play the ready chime, then navigate — via
	// the shared helper so a same-URL target forces a fresh GET (bfcache-safe)
	// rather than a no-op.
	window.handlers.set(redirectTag, (message) => {
		if (message.d && message.d.l) {
			window.notification.play();
			window.navigateTo(message.d.l);
		}
	});

	return true;
});

/**
 * Sends a request for room updates to redirect after the fact if we miss
 * the game ready redirect message
 */
const requestRoomUpdate = () => {
	send(buildCommand(roomUpdateTag, {q: true}))
};
