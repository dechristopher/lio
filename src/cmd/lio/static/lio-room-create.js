const redirectTag = "e";
const roomUpdateTag = "r";

window.addEventListener('load', () => {
	const inviteLink = document.getElementById('gameInviteLink');

	// The QR modal, phones only: from 40rem up the CSS flattens #modalQR with
	// display:contents and hides its button, so the code is already inline and
	// none of this can fire. Adding .open is inert against that flattening
	// because the ID selector outranks .modal-shade.open.
	// The shared pageshow handler in components.templ closes every open
	// .modal-shade on a bfcache restore, so this one needs no reset of its own.
	const qrModal = document.getElementById('modalQR');
	const qrButton = document.getElementById('qrModalButton');
	if (qrModal && qrButton) {
		const closeQR = () => qrModal.classList.remove('open');
		qrButton.onclick = () => qrModal.classList.add('open');
		qrModal.querySelector('.modal-close').onclick = closeQR;
		// backdrop click, but not a click that landed on the modal box itself
		qrModal.addEventListener('click', (e) => { if (e.target === qrModal) closeQR(); });
		document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeQR(); });
	}

	document.getElementById('copyInviteButton').onclick = () => {
		document.getElementById('copyInviteButton').classList.add('copied');
		navigator.clipboard.writeText(inviteLink.value);
	};

	// the OS share sheet: the real way an invite gets sent from a phone. The
	// button ships hidden and is only revealed where navigator.share exists, so
	// browsers without it fall through to the copy button rather than showing a
	// dead control. An aborted share (the user dismissing the sheet) rejects,
	// which is not an error worth surfacing.
	const shareButton = document.getElementById('shareInviteButton');
	if (shareButton && navigator.share) {
		shareButton.hidden = false;
		shareButton.onclick = () => {
			navigator.share({
				title: 'Octad challenge',
				text: 'Join my game on octad.gg',
				url: inviteLink.value,
			}).catch(() => {});
		};
	}

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
