// lio-about.js — /about rules page: three looping mini-boards that play each
// castle type (near / center / far) so the swap-vs-cross mechanics can be seen
// rather than read. Positions are hard-coded OFEN snapshots generated with the
// octad library; octadground only renders them, animating the piece slides
// between snapshots. The about sections are htmx fragments swapped into
// #about-content, so boards are (re)mounted on every swap as well as on load,
// and any demos from a previous swap are torn down first.
(function () {
	'use strict';

	// timing (ms): a beat before the first move, a stride between moves, and a
	// hold on the completed castle before the loop resets.
	const firstMoveDelay = 900;
	const moveDelay = 1400;
	const endHold = 2600;

	// Each step is a board OFEN plus the from/to squares of the move that
	// produced it (octadground's lastMove highlight). Castle steps highlight the
	// king's journey.
	const sequences = {
		near: [
			{o: 'ppkn/4/4/NKPP', m: []},
			{o: 'ppkn/4/4/KNPP', m: ['b1', 'a1']}, // 1. O — king and knight swap
		],
		center: [
			{o: 'ppkn/4/4/NKPP', m: []},
			{o: 'ppkn/4/4/NPKP', m: ['b1', 'c1']}, // 1. O-O — king and c-pawn swap
		],
		far: [
			{o: 'ppkn/4/4/NKPP', m: []},
			{o: 'ppkn/4/2P1/NK1P', m: ['c1', 'c2']}, // 1. c2 clears the way
			{o: 'p1kn/1p2/2P1/NK1P', m: ['b4', 'b3']}, // 1... b3
			{o: 'p1kn/1p2/2P1/NPK1', m: ['b1', 'c1']}, // 2. O-O-O — king and d-pawn cross
		],
	};

	let demos = []; // active {og, timer} instances, torn down before re-init

	const destroyAll = () => {
		demos.forEach((d) => {
			clearTimeout(d.timer);
			try {
				d.og.destroy();
			} catch (e) { /* ignore */ }
		});
		demos = [];
	};

	const mountDemos = () => {
		destroyAll();
		if (typeof Octadground === 'undefined') {
			return;
		}
		document.querySelectorAll('[data-castle-demo]').forEach((mount) => {
			const seq = sequences[mount.dataset.castleDemo];
			if (!seq) {
				return;
			}
			const demo = {timer: null};
			demo.og = Octadground(mount, {
				ofen: seq[0].o,
				orientation: 'white',
				viewOnly: true,
				coordinates: false,
				highlight: {lastMove: true, check: false},
				drawable: {enabled: false},
			});

			let step = 0;
			const advance = () => {
				step++;
				if (step >= seq.length) {
					// reset to the start without animating the pieces back
					step = 0;
					demo.og.set({animation: {enabled: false}, ofen: seq[0].o, lastMove: []});
					demo.og.set({animation: {enabled: true}});
					demo.timer = setTimeout(advance, firstMoveDelay);
					return;
				}
				demo.og.set({ofen: seq[step].o, lastMove: seq[step].m});
				demo.timer = setTimeout(advance, step === seq.length - 1 ? endHold : moveDelay);
			};
			demo.timer = setTimeout(advance, firstMoveDelay);
			demos.push(demo);
		});
	};

	// a hidden tab should not animate: stop everything, restart when visible
	document.addEventListener('visibilitychange', () => {
		if (document.hidden) {
			demos.forEach((d) => clearTimeout(d.timer));
		} else {
			mountDemos();
		}
	});

	// htmx swaps the about sections in place; remount after each swap
	document.body.addEventListener('htmx:afterSwap', mountDemos);
	window.addEventListener('pagehide', destroyAll);

	mountDemos();
})();
