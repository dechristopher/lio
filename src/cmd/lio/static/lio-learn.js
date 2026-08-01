// lio-learn.js — the /learn tutorial client.
//
// The browser has no octad rules engine, so this file owns none of the rules.
// It renders the board, collects input, and asks POST /api/learn what happened;
// the server applies the move, judges it against the lesson's goal, answers for
// the opponent and returns the coach's line. The one exception is the
// board-literacy step, which asks the learner to click named squares — no move
// is made, so there is nothing for the server to judge.
//
// The whole course is inlined as #learn-data, so switching lessons is local:
// the rail rewrites the URL with pushState and re-renders in place. Every lesson
// is still a real server-rendered URL, which is what makes a reload, a
// bookmark, or a shared link land where it should.
//
// Progress is per-device localStorage. The tutorial is aimed at people who do
// not have an account yet, so requiring one to keep your place would defeat it.
(function () {
	'use strict';

	const mount = document.getElementById('learn-board');
	const dataEl = document.getElementById('learn-data');
	if (!mount || !dataEl || typeof Octadground === 'undefined') {
		return;
	}

	let course;
	try {
		course = JSON.parse(dataEl.textContent);
	} catch (e) {
		return;
	}
	if (!course || !Array.isArray(course.lessons) || course.lessons.length === 0) {
		return;
	}

	const progressKey = 'learnDone';
	const api = '/api/learn';
	// how long a mistake stays on the board before the step restarts: long
	// enough to see what the move did, short enough not to feel like a penalty
	const mistakeResetDelay = 1500;

	// ---- sound ----
	// the same three cues the game board gives, defined here rather than pulling
	// in lio-game.js (an order of magnitude larger, and it boots against room
	// markup that does not exist on this page)
	const sfx = {};
	if (typeof Howl !== 'undefined') {
		sfx.move = new Howl({src: ['/res/sfx/move.ogg', '/res/sfx/move.mp3'], preload: true, volume: 0.75});
		sfx.capture = new Howl({src: ['/res/sfx/capture.ogg', '/res/sfx/capture.mp3'], preload: true, volume: 0.9});
		sfx.check = new Howl({src: ['/res/sfx/check.ogg', '/res/sfx/check.mp3'], preload: true, volume: 0.9});
	}
	const play = (name) => {
		const s = sfx[name];
		if (s) {
			try {
				s.play();
			} catch (e) { /* audio is a nicety, never a failure */ }
		}
	};

	// ---- elements ----
	const el = {
		gcon: document.getElementById('learn-gcon'),
		title: document.getElementById('learn-title'),
		dots: document.getElementById('learn-dots'),
		prompt: document.getElementById('learn-prompt'),
		feedback: document.getElementById('learn-feedback'),
		back: document.getElementById('learn-back'),
		show: document.getElementById('learn-show'),
		reset: document.getElementById('learn-reset'),
		next: document.getElementById('learn-next'),
		finished: document.getElementById('learn-finished'),
		annotation: document.getElementById('learn-annotation'),
		deployQuestions: document.getElementById('learn-deploy-questions'),
		deployOverlay: document.getElementById('learn-deploy-overlay'),
		deployConfirm: document.getElementById('learn-deploy-confirm'),
		deployWaiting: document.getElementById('learn-deploy-waiting'),
		promoShade: document.getElementById('learn-promo-shade'),
		promo: document.getElementById('learn-promo'),
		cpFill: document.getElementById('learn-cp-fill'),
		cpText: document.getElementById('learn-cp-text'),
		rail: document.querySelector('.learn-rail'),
		coach: document.querySelector('.learn-coach'),
	};

	// ---- state ----
	let lesson = lessonBySlug(course.start) || course.lessons[0];
	let stepIdx = 0;
	let step = lesson.steps[0];
	let ofen = step.o;          // the position on the board right now
	let played = 0;             // moves spent on this step
	let stepDone = false;       // goal met; the board is frozen until Next
	let busy = false;           // a request is in flight
	// a "Show me" demonstration is playing: the board stays locked so a stray
	// drag cannot interleave with the moves being replayed
	let demoing = false;
	let hits = [];              // squares already found in a click-the-square step
	// the learner's moves so far match the step's solution, so the next solution
	// move is still the right thing to point at
	let onPath = true;
	// bumped on every step entry; a deferred auto-reset checks it so a lesson
	// switch during the pause cannot reset the step the learner has moved on to
	let stepToken = 0;

	function lessonBySlug(slug) {
		return course.lessons.find((l) => l.slug === slug) || null;
	}

	// ---- progress (per device) ----
	function doneSet() {
		try {
			const raw = localStorage.getItem(progressKey);
			const list = raw ? JSON.parse(raw) : [];
			return new Set(Array.isArray(list) ? list : []);
		} catch (e) {
			return new Set();
		}
	}
	function markDone(slug) {
		const set = doneSet();
		if (set.has(slug)) {
			return;
		}
		set.add(slug);
		try {
			localStorage.setItem(progressKey, JSON.stringify(Array.from(set)));
		} catch (e) { /* private browsing: the lesson still works, it just won't persist */ }
	}
	function renderProgress() {
		const set = doneSet();
		document.querySelectorAll('.learn-lesson').forEach((a) => {
			a.classList.toggle('done', set.has(a.dataset.lesson));
			a.classList.toggle('current', a.dataset.lesson === lesson.slug);
		});
		const total = course.lessons.length;
		const done = course.lessons.filter((l) => set.has(l.slug)).length;
		if (el.cpFill) {
			el.cpFill.style.width = total ? ((done / total) * 100) + '%' : '0';
		}
		if (el.cpText) {
			el.cpText.textContent = done + ' of ' + total + ' lessons';
		}
		if (el.finished) {
			el.finished.classList.toggle('hidden', done < total);
		}
	}

	// ---- board ----
	const boardOf = (o) => (o || '').split(' ')[0];
	const turnOf = (o) => ((o || '').split(' ')[1] === 'b' ? 'black' : 'white');

	// Configured to match the game board (lio-game.js) as closely as a boardless
	// lesson can, so the drag/tap feel a learner practises here is the feel they
	// get in a real game. Two deliberate differences:
	//
	//   selectable is touch-only, exactly as the room has it. On desktop the
	//   board is drag-only; leaving click-to-select on made a click arm a
	//   half-move that a following click completed, which reads as a piece that
	//   "did not get grabbed". events.select still fires on every click (board
	//   .selectSquare is called before this flag is consulted), so the
	//   click-a-square lesson works on desktop regardless.
	//
	//   premoves are off. The server judges every move, so a queued premove
	//   would fire a move the lesson never asked for the moment the board is
	//   re-armed.
	const og = Octadground(mount, {
		ofen: boardOf(step.o),
		orientation: 'white',
		coordinates: true,
		highlight: {lastMove: true, check: true},
		movable: {free: false, color: undefined, dests: new Map()},
		draggable: {enabled: true},
		premovable: {enabled: false},
		selectable: {enabled: !!window.isMobile},
		events: {
			move: onBoardMove,
			select: onBoardSelect,
		},
	});

	// octadground caches the board's bounding rect and only invalidates it on
	// scroll, window resize, and a ResizeObserver on the board itself. None of
	// those fire when the board is *moved* without changing size — which is
	// exactly what the single-column layout does every time the coach panel
	// above it grows or shrinks (a longer prompt, a feedback line appearing).
	// The cached rect is then stale by the height difference and every dragged
	// piece hangs off the cursor by that much. Watching the elements that can
	// push the board around and re-using octadground's own resize path is the
	// cheapest correct fix.
	if ('ResizeObserver' in window) {
		const invalidate = new ResizeObserver(() => {
			window.dispatchEvent(new Event('resize'));
		});
		[el.coach, el.finished, document.querySelector('.learn-head')]
			.forEach((node) => {
				if (node) {
					invalidate.observe(node);
				}
			});
	}

	// dests arrive from the server as a plain object; octadground wants a Map
	function destMap(v) {
		const m = new Map();
		Object.keys(v || {}).forEach((k) => m.set(k, v[k]));
		return m;
	}

	// The learner is always White in this course. Every lesson position is
	// authored White-to-move, which keeps "your pieces are the ones at the
	// bottom" true on every board a beginner sees.
	function movableFor(o, dests) {
		if (stepDone || demoing || turnOf(o) !== 'white') {
			return {free: false, color: undefined, dests: new Map()};
		}
		return {free: false, color: 'white', dests: destMap(dests)};
	}

	// ---- coach panel ----
	function say(text, tone) {
		if (!el.feedback) {
			return;
		}
		el.feedback.textContent = text || '';
		el.feedback.classList.remove('good', 'bad');
		if (tone) {
			el.feedback.classList.add(tone);
		}
	}

	// The prompt is two nodes: the teaching, then the instruction accented so it
	// survives a skim of the paragraph. Built with text nodes rather than
	// innerHTML — this is curriculum copy, but it is still the one place on the
	// page where server text becomes markup.
	function renderPrompt() {
		if (!el.prompt) {
			return;
		}
		el.prompt.textContent = '';
		el.prompt.appendChild(document.createTextNode(step.prompt || ''));
		if (step.action) {
			el.prompt.appendChild(document.createTextNode(' '));
			const action = document.createElement('strong');
			action.id = 'learn-action';
			action.className = 'learn-action';
			action.textContent = step.action;
			el.prompt.appendChild(action);
		}
		if (step.moves === 1) {
			const badge = document.createElement('span');
			badge.className = 'learn-badge';
			badge.textContent = 'in one move';
			el.prompt.appendChild(badge);
		}
	}

	function renderDots() {
		if (!el.dots) {
			return;
		}
		el.dots.textContent = '';
		lesson.steps.forEach((_, i) => {
			const dot = document.createElement('span');
			dot.className = 'learn-dot' + (i === stepIdx ? ' current' : (i < stepIdx ? ' done' : ''));
			el.dots.appendChild(dot);
		});
	}

	function renderActions() {
		const isDeploy = lesson.kind === 'deploy';
		const isSelect = step.goal === 'select';
		// "Show me" plays the answer back; a deploy step has no single answer,
		// and a full game has no answer at all
		toggle(el.show, !isDeploy && !isSelect && !stepDone && !demoing
			&& (step.solution || []).length > 0);
		toggle(el.reset, !demoing);
		toggle(el.next, stepDone);
		if (el.back) {
			// disabled in place rather than hidden: it is the one control that
			// is always relevant, and the row should not change shape around it
			el.back.disabled = atCourseStart() || demoing;
			el.back.title = stepIdx > 0 ? 'Previous step' : 'Previous lesson';
		}
		if (el.next) {
			el.next.textContent = lastStep() ? (lastLesson() ? 'Finish' : 'Next lesson →') : 'Next →';
		}
	}

	function toggle(node, on) {
		if (node) {
			node.classList.toggle('hidden', !on);
		}
	}

	const lastStep = () => stepIdx >= lesson.steps.length - 1;
	const lastLesson = () => course.lessons.indexOf(lesson) >= course.lessons.length - 1;

	// ---- board annotations ----
	// Both the coach's marks and the click-the-square markers are octadground
	// auto-shapes, drawn into the board's own SVG layer. That layer exists
	// whenever drawable.visible is set (it is by default, independently of
	// drawable.enabled, which only gates user drawing), so the marks align to the
	// squares exactly and under every board theme, with no DOM of ours inside the
	// element octadground owns and re-renders.
	//
	// The move marks are *derived from the step's solution*, never hand-drawn
	// per step. The solution is the one thing about a step that is proved
	// correct — TestCurriculum replays it through the real judge — so deriving
	// from it is the only way the circled piece and the arrow cannot drift from
	// the move the lesson actually wants. Hand-authored arrows had already
	// drifted once: the far-castle step drew the king landing on c1 while the
	// move to play is the king onto the d1 partner.
	function solutionShapes() {
		const uoi = (step.solution || [])[played];
		if (!uoi || uoi.length < 4) {
			return [];
		}
		const orig = uoi.slice(0, 2);
		const dest = uoi.slice(2, 4);
		// a circle on the piece to move, and an arrow to where it goes
		return [
			{orig: orig, brush: 'green'},
			{orig: orig, dest: dest, brush: 'green'},
		];
	}

	// The opponent move that produced the position is shown the same way a real
	// game shows the move just played: octadground's own lastMove square
	// highlight (priorLastMove, applied when the step opens). That is the
	// vocabulary a learner will meet everywhere else on the site, so it is the
	// right default and costs no extra marks on a 4x4 board.
	//
	// priorShapes adds a blue arrow on top, but only for a move that *skipped
	// over* a square. Two highlighted squares fully describe a step between
	// neighbours; they cannot show the ground a double push covered, which is
	// the entire point of en passant — the capture lands on a square the pawn
	// merely passed through. Dropped once the learner moves, because from then
	// on it describes a board that no longer exists.
	function priorShapes() {
		const uoi = step.prior;
		if (!uoi || uoi.length < 4 || played > 0) {
			return [];
		}
		const orig = uoi.slice(0, 2);
		const dest = uoi.slice(2, 4);
		const files = Math.abs(orig.charCodeAt(0) - dest.charCodeAt(0));
		const ranks = Math.abs(+orig[1] - +dest[1]);
		if (Math.max(files, ranks) < 2) {
			return []; // adjacent: the square highlight already says it all
		}
		return [{orig: orig, dest: dest, brush: 'blue'}];
	}

	// priorLastMove is the prior move as octadground's lastMove pair, so the
	// board's existing .last-move square styling does the work.
	function priorLastMove() {
		const uoi = step.prior;
		if (!uoi || uoi.length < 4) {
			return [];
		}
		return [uoi.slice(0, 2), uoi.slice(2, 4)];
	}

	function renderShapes() {
		if (step.goal === 'select') {
			// blue for a square still to find, green once it has been found
			og.setAutoShapes((step.targets || []).map((sq) => ({
				orig: sq,
				brush: hits.indexOf(sq) >= 0 ? 'green' : 'blue',
			})));
			return;
		}
		// Once the learner has left the scripted line the remaining solution
		// moves describe a position that is no longer on the board, so there is
		// nothing honest to point at.
		og.setAutoShapes(priorShapes().concat(onPath ? solutionShapes() : []));
	}

	function clearShapes() {
		og.setAutoShapes([]);
	}

	// ---- lesson / step lifecycle ----
	// atStep is where to open the lesson; it defaults to the first step, and is
	// only ever passed when walking backwards into the end of a previous lesson.
	function openLesson(slug, pushHistory, atStep) {
		const next = lessonBySlug(slug);
		if (!next) {
			return;
		}
		lesson = next;
		if (pushHistory) {
			try {
				history.pushState({lesson: slug}, '', '/learn/' + slug);
			} catch (e) { /* history is optional; the lesson still opens */ }
		}
		if (el.title) {
			el.title.textContent = lesson.title;
		}
		const i = (typeof atStep === 'number' && atStep >= 0 && atStep < lesson.steps.length)
			? atStep : 0;
		openStep(i);
		renderProgress();
	}

	// goBack walks the course in reverse: the previous step, or — at the first
	// step of a lesson — the last step of the lesson before it, so back and
	// forward cover the same ground.
	function goBack() {
		if (demoing) {
			return;
		}
		if (stepIdx > 0) {
			openStep(stepIdx - 1);
			return;
		}
		const i = course.lessons.indexOf(lesson);
		if (i <= 0) {
			return; // the very start of the course; the button is disabled here
		}
		const prev = course.lessons[i - 1];
		openLesson(prev.slug, true, prev.steps.length - 1);
	}

	// atCourseStart reports the one place there is nothing to go back to.
	const atCourseStart = () =>
		stepIdx === 0 && course.lessons.indexOf(lesson) === 0;

	function openStep(i) {
		stepIdx = i;
		step = lesson.steps[i];
		ofen = step.o;
		played = 0;
		stepDone = false;
		demoing = false;
		onPath = true;
		stepToken++;
		hits = [];
		hidePromo();
		hideAnnotation();
		hideDeployOverlays();

		// The starting position's legal moves ship with the curriculum, so the
		// board is armed in the same frame the step opens. It used to wait on a
		// describe round trip, which left a window where clicking a piece did
		// nothing at all.
		const armed = (lesson.kind === 'deploy' || step.goal === 'select')
			? {free: false, color: undefined, dests: new Map()}
			: movableFor(ofen, step.v);

		og.set({
			animation: {enabled: false},
			ofen: boardOf(ofen),
			orientation: 'white',
			turnColor: turnOf(ofen),
			// the opponent's move into this position, highlighted exactly as a
			// real game highlights the move just played (see priorLastMove).
			// The learner's own first move replaces it, as it would in a game.
			lastMove: priorLastMove(),
			check: false,
			movable: armed,
			// octadground's own castle guess is wanted while playing: dragging
			// the king onto a partner renders the castle immediately instead of
			// showing a vanished partner until the server's position arrives.
			// enterDeploy turns it back off — see there.
			autoCastle: true,
		});
		og.set({animation: {enabled: true}});
		// the board just changed what it is showing, and on a narrow layout the
		// coach panel above it may be about to resize; make sure the drag
		// geometry is measured against where the board actually is
		refreshBounds();

		renderPrompt();
		renderDots();
		say('');
		renderShapes();
		renderActions();

		if (lesson.kind === 'deploy') {
			enterDeploy();
		}
	}

	// refreshBounds drops octadground's cached board rect via its own resize
	// path, so the next drag measures against the board's current position.
	function refreshBounds() {
		window.dispatchEvent(new Event('resize'));
	}

	function advance() {
		// Next is hidden until the step is passed, so a user cannot reach this
		// early — but nothing else guarantees it, and advancing on an unfinished
		// step would skip the lesson silently.
		if (!stepDone) {
			return;
		}
		if (!lastStep()) {
			openStep(stepIdx + 1);
			return;
		}
		markDone(lesson.slug);
		renderProgress();
		const i = course.lessons.indexOf(lesson);
		if (i >= 0 && i < course.lessons.length - 1) {
			openLesson(course.lessons[i + 1].slug, true);
			return;
		}
		// end of the course: the completion card is already revealed by
		// renderProgress, so just settle the panel on it
		say('That is the whole course. Well done.', 'good');
		renderActions();
		if (el.finished) {
			el.finished.scrollIntoView({behavior: 'smooth', block: 'nearest'});
		}
	}

	// ---- API ----
	function request(body) {
		busy = true;
		return fetch(api, {
			method: 'POST',
			headers: {'Content-Type': 'application/json'},
			body: JSON.stringify(body),
		}).then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
			.then((res) => {
				busy = false;
				return res;
			})
			.catch(() => {
				busy = false;
				say('Could not reach the server — check your connection and try again.', 'bad');
				return null;
			});
	}

	// ---- board input ----
	function onBoardSelect(square) {
		if (step.goal !== 'select' || stepDone) {
			return;
		}
		const targets = step.targets || [];
		if (targets.indexOf(square) < 0) {
			say('Not that one. ' + (step.hint || ''), 'bad');
			return;
		}
		if (hits.indexOf(square) < 0) {
			hits.push(square);
		}
		renderShapes();
		play('move');
		if (hits.length >= targets.length) {
			// a click-the-square step is judged here, so its success line is the
			// one the server sends it down with (see LearnStep.Success)
			finishStep(step.success);
		}
	}

	function onBoardMove(orig, dest) {
		if (stepDone || busy || demoing) {
			// put the board back: the move never happened
			og.set({ofen: boardOf(ofen), turnColor: turnOf(ofen)});
			return;
		}
		if (lesson.kind === 'deploy') {
			onDeploySwap(orig, dest);
			return;
		}
		// a promotion push needs the piece chosen before the move is a move;
		// octadground has already placed the pawn on the final rank
		const piece = og.state.pieces.get(dest);
		if (piece && piece.role === 'pawn' && piece.color === 'white' && dest[1] === '4') {
			showPromo(orig, dest);
			return;
		}
		sendMove(orig + dest);
	}

	function sendMove(uoi) {
		og.set({movable: {free: false, color: undefined, dests: new Map()}});
		request({
			lesson: lesson.slug,
			step: stepIdx,
			ofen: ofen,
			uoi: uoi,
			played: played,
		}).then((res) => {
			if (!res) {
				// the move did not happen: restore the position we were showing
				og.set({ofen: boardOf(ofen), turnColor: turnOf(ofen)});
				refreshDests();
				return;
			}
			played++;
			applyResult(res);
		});
	}

	// applyResult renders a judged move: the learner's move, then the opponent's
	// answer a beat later so the two read as separate events rather than one
	// board jump.
	function applyResult(res) {
		if (res.mv) {
			showMove(res.mv);
		}
		const settle = () => {
			if (res.rp) {
				showMove(res.rp);
			}
			finishResult(res);
		};
		if (res.mv && res.rp) {
			setTimeout(settle, 420);
		} else {
			settle();
		}
	}

	function showMove(mv) {
		og.set({
			ofen: boardOf(mv.o),
			turnColor: turnOf(mv.o),
			lastMove: mv.lm || [],
			check: !!mv.k,
		});
		play(mv.k ? 'check' : (mv.x ? 'capture' : 'move'));
	}

	function finishResult(res) {
		ofen = res.o || ofen;
		og.set({
			ofen: boardOf(ofen),
			turnColor: turnOf(ofen),
			check: !!res.k,
		});
		if (res.over) {
			showAnnotation(res);
		}
		if (res.done) {
			finishStep(res.say);
			return;
		}

		// track whether the learner is still walking the scripted line; the
		// move marks follow it, and stepping off is what "the wrong piece" means
		const expected = (step.solution || [])[played - 1];
		if (onPath && (!res.mv || !expected || res.mv.uoi !== expected)) {
			onPath = false;
		}

		if (res.failed) {
			restartAfterMistake(res.say || 'Not this time — here it is again.');
			return;
		}
		// the wrong piece off the very first move: the lesson is about one piece
		// and they picked another, so put it back rather than letting them drift
		// further from a position the coaching still describes
		if (played === 1 && !onPath && wrongPieceMoved(res)) {
			restartAfterMistake(step.hint || 'Not that piece — try the one the arrow points at.');
			return;
		}

		say(res.say || '', '');
		renderShapes();
		og.set({movable: movableFor(ofen, res.v)});
	}

	// wrongPieceMoved reports whether the move started from a different square
	// than the step's solution does. Only meaningful on the first move, which is
	// the only point at which the solution's origin is guaranteed to describe
	// the position the learner is looking at.
	function wrongPieceMoved(res) {
		const sol = (step.solution || [])[0];
		if (!sol || !res.mv || !res.mv.uoi) {
			return false;
		}
		return res.mv.uoi.slice(0, 2) !== sol.slice(0, 2);
	}

	// restartAfterMistake puts the step back the way it started and re-draws the
	// marks, after a beat long enough to see what went wrong. The coach's line
	// survives the reset — the explanation is the point, and openStep clears it.
	// The graduation game is exempt: it is a real game, and losing one is not a
	// mistake to be rewound out from under the player.
	function restartAfterMistake(message) {
		stepDone = false;
		og.set({movable: {free: false, color: undefined, dests: new Map()}});
		say(message, 'bad');
		renderActions();
		if (lesson.kind === 'play') {
			return;
		}
		const token = stepToken;
		setTimeout(() => {
			if (token !== stepToken) {
				return; // they moved on while the pause ran
			}
			openStep(stepIdx);
			say(message, 'bad');
		}, mistakeResetDelay);
	}

	function finishStep(text) {
		stepDone = true;
		og.set({movable: {free: false, color: undefined, dests: new Map()}});
		clearShapes();
		say(text || 'Done.', 'good');
		renderDots();
		renderActions();
		play('move');
		if (lastStep()) {
			markDone(lesson.slug);
			renderProgress();
		}
	}

	function refreshDests() {
		request({lesson: lesson.slug, step: stepIdx, ofen: ofen}).then((res) => {
			if (res) {
				og.set({movable: movableFor(res.o, res.v)});
			}
		});
	}

	// ---- promotion picker ----
	function showPromo(orig, dest) {
		if (!el.promo || !el.promoShade) {
			sendMove(orig + dest + 'q');
			return;
		}
		el.promoShade.classList.remove('hidden');
		el.promo.classList.remove('hidden');
		el.promo.classList.add('f' + dest[0]);
		// clone-replace first, so a previous promotion's listeners are gone
		const stale = el.promo.getElementsByTagName('piece');
		for (let i = stale.length - 1; i >= 0; i--) {
			stale[i].replaceWith(stale[i].cloneNode(true));
		}
		const pieces = el.promo.getElementsByTagName('piece');
		for (let i = 0; i < pieces.length; i++) {
			const node = pieces[i];
			node.classList.add('white');
			const promo = node.classList.contains('queen') ? 'q'
				: node.classList.contains('rook') ? 'r'
					: node.classList.contains('bishop') ? 'b' : 'n';
			node.addEventListener('click', () => {
				hidePromo(dest);
				sendMove(orig + dest + promo);
			});
		}
	}

	function hidePromo(dest) {
		if (!el.promo || !el.promoShade) {
			return;
		}
		el.promoShade.classList.add('hidden');
		el.promo.classList.add('hidden');
		if (dest) {
			el.promo.classList.remove('f' + dest[0]);
		} else {
			['a', 'b', 'c', 'd'].forEach((f) => el.promo.classList.remove('f' + f));
		}
		const pieces = el.promo.getElementsByTagName('piece');
		for (let i = 0; i < pieces.length; i++) {
			pieces[i].classList.remove('white', 'black');
		}
	}

	// ---- result pill ----
	const reasons = {
		checkmate: 'by checkmate',
		stalemate: 'by stalemate',
		repetition: 'by repetition',
		moverule: 'by the 25-move rule',
		insufficient: 'by insufficient material',
	};
	function showAnnotation(res) {
		if (!el.annotation) {
			return;
		}
		const method = reasons[res.rr] || '';
		let text;
		if (res.over === 'd') {
			text = method ? ('Draw ' + method) : 'Draw';
		} else {
			text = (res.over === 'w' ? 'White wins' : 'Black wins') + (method ? ' ' + method : '');
		}
		el.annotation.textContent = text;
		el.annotation.classList.add('ea-show');
	}
	function hideAnnotation() {
		if (el.annotation) {
			el.annotation.classList.remove('ea-show');
			el.annotation.textContent = '';
		}
	}

	// ---- deploy lesson ----
	// A faithful copy of the live blind-deploy phase, because a lesson that
	// showed a simplified version of it would teach the wrong thing. That means:
	// the learner sees only their own four pieces, the opponent's home rank sits
	// behind an opaque "?" band until the reveal, swaps happen within the home
	// rank as often as the learner likes, and confirming is a button on the
	// board. The one live detail deliberately left out is the 30-second timer:
	// it exists to stop a real game stalling, and here it would only rush
	// somebody who is still reading.
	const homeRank = ['a1', 'b1', 'c1', 'd1'];

	// deployArrangement is square -> piece for the learner's home rank. It is
	// tracked rather than read back off the board because octadground treats a
	// swap as an ordinary move and *overwrites* the destination piece — reading
	// the board back after a swap loses whatever used to stand there. The live
	// client keeps the same model for the same reason.
	let deployArrangement = null;

	const deployMovable = () => {
		const dests = new Map();
		homeRank.forEach((s) => dests.set(s, homeRank.filter((x) => x !== s)));
		return {free: false, color: 'white', dests: dests};
	};

	// the learner's own pieces only; the opponent's rank is blank underneath the
	// "?" band and is filled in by the reveal
	const deployStartBoard = '4/4/4/NKPP';

	function enterDeploy() {
		og.set({
			animation: {enabled: false},
			ofen: deployStartBoard,
			turnColor: 'white',
			lastMove: [],
			check: false,
			movable: deployMovable(),
			// Arranging is not playing, and octadground cannot tell the
			// difference: dragging the king onto a partner two or more files
			// away looks exactly like a far castle to it, so it relocates both
			// pieces itself — *after* handing the raw squares to events.move.
			// onDeploySwap then reconstructs the swap on top of an already
			// rearranged board and the king ends up on two squares at once. The
			// live client disables it here for the same reason.
			autoCastle: false,
		});
		og.set({animation: {enabled: true}});

		deployArrangement = new Map();
		homeRank.forEach((sq) => deployArrangement.set(sq, og.state.pieces.get(sq)));

		show(el.deployQuestions, true);
		if (el.deployQuestions) {
			el.deployQuestions.classList.remove('deploy-reveal');
		}
		show(el.deployOverlay, true);
		toggle(el.deployWaiting, false);
		if (el.deployConfirm) {
			el.deployConfirm.disabled = false;
		}
		say('');
	}

	// show/hide the room's .deploy-show overlays
	function show(node, on) {
		if (node) {
			node.classList.toggle('deploy-show', on);
		}
	}

	function hideDeployOverlays() {
		show(el.deployQuestions, false);
		show(el.deployOverlay, false);
		if (el.deployQuestions) {
			el.deployQuestions.classList.remove('deploy-reveal');
		}
		deployArrangement = null;
	}

	// onDeploySwap reconstructs a swap out of the move octadground just made.
	// The re-render is deferred a tick so octadground finishes its own drag
	// handling first, and the displaced piece is seeded back onto the
	// destination before setPieces so it animates sliding across rather than
	// popping into place — both lifted from the live client, which learned them
	// the hard way.
	function onDeploySwap(orig, dest) {
		if (!deployArrangement) {
			return;
		}
		const moved = deployArrangement.get(orig);
		const displaced = deployArrangement.get(dest);
		if (!moved) {
			return;
		}
		deployArrangement.set(dest, moved);
		deployArrangement.set(orig, displaced);
		play('move');
		setTimeout(() => {
			if (displaced) {
				og.state.pieces.set(dest, displaced);
			} else {
				og.state.pieces.delete(dest);
			}
			og.state.pieces.delete(orig);
			og.setPieces(new Map([[dest, moved], [orig, displaced || null]]));
			// octadground flips turnColor after a user move; restore it so the
			// next swap is allowed, and re-assert the home-rank restriction
			og.set({turnColor: 'white', movable: deployMovable()});
		}, 0);
	}

	// deployOrderString reads the tracked arrangement as the four piece letters
	// the API wants, in the learner's own left-to-right order.
	function deployOrderString() {
		if (!deployArrangement) {
			return '';
		}
		const order = homeRank.map((sq) => {
			const piece = deployArrangement.get(sq);
			if (!piece) {
				return '';
			}
			return piece.role === 'king' ? 'k' : (piece.role === 'knight' ? 'n' : 'p');
		});
		return order.some((ch) => ch === '') ? '' : order.join('');
	}

	function commitDeploy() {
		const order = deployOrderString();
		if (!order || stepDone || busy) {
			return;
		}
		// lock the board and sit in the "waiting for opponent" beat a real
		// deploy has — the reveal only means something if the arranging was
		// visibly blind up to it
		og.set({movable: {free: false, color: undefined, dests: new Map()}});
		if (el.deployConfirm) {
			el.deployConfirm.disabled = true;
		}
		toggle(el.deployWaiting, true);

		request({lesson: lesson.slug, step: stepIdx, deploy: order}).then((res) => {
			if (!res) {
				// let them try again rather than stranding a locked board
				if (el.deployConfirm) {
					el.deployConfirm.disabled = false;
				}
				toggle(el.deployWaiting, false);
				og.set({movable: deployMovable()});
				return;
			}
			setTimeout(() => revealDeploy(res), 700);
		});
	}

	// revealDeploy drops both setups at once: the opponent's pieces render under
	// the "?" band while it fades, which is what the live reveal looks like.
	function revealDeploy(res) {
		ofen = res.o;
		og.set({
			ofen: boardOf(ofen),
			turnColor: turnOf(ofen),
			lastMove: [],
			movable: {free: false, color: undefined, dests: new Map()},
		});
		if (el.deployQuestions) {
			el.deployQuestions.classList.add('deploy-reveal');
		}
		show(el.deployOverlay, false);
		play('capture'); // the reveal deserves more than a move tick
		// the band's opacity transition is 450ms; take it out of the layout once
		// it has finished rather than leaving an invisible element over the rank
		setTimeout(() => show(el.deployQuestions, false), 500);
		finishStep(res.say);
	}

	// ---- "Show me" ----
	// Plays the step's own solution back, one move at a time, through exactly the
	// same path a learner's move takes — so the coach's lines, the sounds and the
	// completion all happen as if it had been played by hand.
	//
	// It always restarts the step first. A solution is only legal from the
	// position the step begins in, and the common reason somebody asks to be
	// shown is that they just played something else — leaving the board on the
	// post-mistake position made every one of those requests an illegal move the
	// server rejected, surfacing as a bogus "could not reach the server".
	// Restarting also returns the move budget, which a failed attempt had spent.
	function showSolution() {
		if ((step.solution || []).length === 0 || busy || demoing) {
			return;
		}
		openStep(stepIdx);
		const moves = (step.solution || []).slice();
		demoing = true;
		say('Watch.', '');
		renderActions();
		const playNext = () => {
			if (moves.length === 0 || !demoing) {
				return;
			}
			const uoi = moves.shift();
			request({lesson: lesson.slug, step: stepIdx, ofen: ofen, uoi: uoi, played: played})
				.then((res) => {
					if (!res || !demoing) {
						demoing = false;
						renderActions();
						return;
					}
					played++;
					applyResult(res);
					if (moves.length > 0) {
						setTimeout(playNext, 900);
					} else {
						// outlast applyResult's opponent-reply settle, so the
						// controls do not come back while pieces are still moving
						setTimeout(() => {
							demoing = false;
							renderActions();
						}, 520);
					}
				});
		};
		setTimeout(playNext, 450);
	}

	// ---- controls ----
	if (el.next) {
		el.next.addEventListener('click', advance);
	}
	if (el.back) {
		el.back.addEventListener('click', goBack);
	}
	if (el.reset) {
		el.reset.addEventListener('click', () => openStep(stepIdx));
	}
	if (el.show) {
		el.show.addEventListener('click', showSolution);
	}
	if (el.deployConfirm) {
		el.deployConfirm.addEventListener('click', commitDeploy);
	}

	// the rail swaps lessons in place; every entry stays a real link, so
	// middle-click, ctrl-click and "open in new tab" keep working
	if (el.rail) {
		el.rail.addEventListener('click', (e) => {
			const link = e.target.closest('.learn-lesson');
			if (!link || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) {
				return;
			}
			e.preventDefault();
			openLesson(link.dataset.lesson, true);
		});
	}

	window.addEventListener('popstate', () => {
		const slug = location.pathname.replace(/^\/learn\/?/, '');
		openLesson(slug || course.lessons[0].slug, false);
	});

	// a back-navigation restores the page with whatever was open; reset the
	// transient board state so a restored page is never mid-lesson-with-no-JS
	window.addEventListener('pageshow', (e) => {
		if (e.persisted) {
			hidePromo();
			openStep(stepIdx);
		}
	});

	// ---- boot ----
	renderProgress();
	openStep(0);
})();
