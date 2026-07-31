// lio-home.js — the home page's activity region, streamed.
//
// The region (#home-activity: stat tiles, open challenges, players panel) is
// server-rendered once by view.HomeActivity and updated from then on by the
// digest frames this file applies (arch/HOME_ACTIVITY_STREAMING.md). It used to
// re-fetch itself from /home/activity every five seconds via htmx, which
// re-derived the whole site picture once per viewer — the presence walk alone
// was O(viewers²) — and destroyed the subtree on every swap.
//
// It owns no socket. lio-tv.js holds the page's single /socket/home connection
// and hands 'hm' frames here through window.lioHomeActivity.
//
// The markup built below mirrors view/home.templ one component at a time
// (challengeRow, playerChip). The duplication is real and is the known cost of
// the design; keep the pairs in step, and prefer patching the smallest node
// that can change over rebuilding a card.
(function () {
	const region = document.getElementById('home-activity');
	if (!region) {
		return;
	}

	const el = (id) => document.getElementById(id);

	// The viewer, from the per-socket hello. name is "" for an anonymous
	// visitor; challenge is whether they may send a challenge at all (signed in,
	// accounts enabled). Both arrive before the first roster frame, because the
	// hub puts Self in the same connect payload.
	let self = {name: '', challenge: false};
	// The viewer's followed players who are online, as last pushed. Held so a
	// roster frame can exclude them without waiting for a following frame.
	let following = [];

	const show = (node, on) => {
		if (node) {
			node.hidden = !on;
		}
	};

	const text = (node, s) => {
		// only touch the DOM when the value actually changed: a no-op write
		// still invalidates and can interrupt a selection
		if (node && node.textContent !== s) {
			node.textContent = s;
		}
	};

	// ---- small builders, mirroring the templ components ----

	const setAttrs = (node, attrs) => {
		Object.keys(attrs).forEach((k) => {
			const v = attrs[k];
			if (v !== null && v !== undefined && v !== false) {
				node.setAttribute(k, v === true ? '' : String(v));
			}
		});
		return node;
	};

	const make = (tag, cls, attrs) => {
		const node = document.createElement(tag);
		if (cls) {
			node.className = cls;
		}
		return attrs ? setAttrs(node, attrs) : node;
	};

	// playerTitle: the account's title badge, absent when untitled
	const titleBadge = (code, tooltip) => {
		if (!code) {
			return null;
		}
		const span = make('span', 'player-title', {title: tooltip || code});
		span.textContent = code;
		return span;
	};

	const svg = (markup) => {
		const wrap = document.createElement('div');
		// static markup defined in this file — no interpolation, so nothing here
		// can carry server or user content
		wrap.innerHTML = markup;
		return wrap.firstElementChild;
	};

	// iconSwords (view/notifications.templ) — the challenge glyph
	const swordsMarkup =
		'<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
		'stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
		'<path d="M14.5 17.5 3 6V3h3l11.5 11.5"></path><path d="M13 19l6-6"></path>' +
		'<path d="M16 16l4 4"></path><path d="M19 21l2-2"></path>' +
		'<path d="M9.5 17.5 21 6V3h-3L6.5 14.5"></path><path d="M11 19l-6-6"></path>' +
		'<path d="M8 16l-4 4"></path><path d="M5 21l-2-2"></path></svg>';

	// iconTrophy (view/components.templ) at the rated badge's size
	const trophyMarkup =
		'<svg class="h-2.5 w-2.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
		'stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
		'<path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6"></path>' +
		'<path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18"></path><path d="M4 22h16"></path>' +
		'<path d="M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22"></path>' +
		'<path d="M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22"></path>' +
		'<path d="M18 2H6v7a6 6 0 0 0 12 0V2Z"></path></svg>';

	// challengeButton(username, "roster-challenge")
	const challengeButton = (name) => {
		const btn = make('button', 'roster-challenge', {
			type: 'button',
			'data-open-create-game': true,
			'data-challenge': name,
			'aria-label': 'Challenge ' + name,
			title: 'Challenge ' + name,
		});
		btn.appendChild(svg(swordsMarkup));
		return btn;
	};

	// canChallenge (view/notifications.go): a viewer may challenge somebody who
	// is not busy and is not themselves. The viewer half of the test arrived on
	// the Self frame.
	const canChallenge = (m) =>
		self.challenge &&
		!m.b &&
		!!m.n &&
		m.n.toLowerCase() !== (self.name || '').toLowerCase();

	// playerChip (view/home.templ) — the one chip builder, shared by every list
	// in the players card. A followed player, a stranger and a new arrival are
	// the same object; the only difference is which list surfaced them.
	//
	// online decides whether the presence dot renders. On the two rosters it is
	// always true; in Arrivals most rows are people who registered and left, and
	// a dot on every chip would be a marker that usually says nothing.
	//
	// when / whenTitle are the Arrivals join token and its tooltip, absent on
	// the rosters.
	const playerChip = (m, online, when, whenTitle) => {
		const li = make('li', 'roster-item');
		let cls = 'roster-chip';
		if (m.p) {
			cls += ' is-playing';
		}
		if (m.b) {
			cls += ' is-busy';
		}
		const attrs = {href: '/@/' + m.n};
		if (whenTitle) {
			attrs.title = whenTitle;
		}
		const a = make('a', cls, attrs);
		if (online) {
			a.appendChild(make('span', 'roster-dot', {'aria-hidden': 'true'}));
		}
		const badge = titleBadge(m.t, m.tn);
		if (badge) {
			a.appendChild(badge);
		}
		const name = make('span', 'roster-chip-name');
		name.textContent = m.n;
		a.appendChild(name);
		// only the unavailable members carry a tag, so it reads as a distinction
		// rather than noise on every chip — and it says why the sword is missing
		if (m.p || m.b) {
			const tag = make('span', 'roster-chip-tag');
			tag.textContent = m.p ? 'playing' : 'waiting';
			a.appendChild(tag);
		}
		if (when) {
			const w = make('span', 'roster-chip-when');
			w.textContent = when;
			a.appendChild(w);
		}
		li.appendChild(a);
		// absent for anybody the dot has turned amber, and for an arrival who is
		// not here at all: a control that is present but fails is worse than one
		// that is absent
		if (online && canChallenge(m)) {
			li.appendChild(challengeButton(m.n));
		}
		return li;
	};

	// everybody in the two rosters is online by definition — that is what being
	// in them means
	const rosterChip = (m) => playerChip(m, true, '', '');

	// arrivalChip: Ago and Joined are pre-formatted server-side, so this file
	// never words a relative time itself.
	const arrivalChip = (m) => playerChip(m, !!m.o, m.a, m.j);

	// colorDot (view/home.templ): the side a joiner would take
	const colorDot = (color) => {
		const base = 'inline-block h-3.5 w-3.5 rounded-full border border-line-strong ';
		if (color === 'w') {
			return make('span', base + 'bg-white', {title: 'White', 'aria-label': 'plays White'});
		}
		if (color === 'b') {
			return make('span', base + 'bg-stone-900', {title: 'Black', 'aria-label': 'plays Black'});
		}
		return make('span', base + 'bg-[linear-gradient(90deg,#ffffff_0_50%,#1c1917_50%_100%)]', {
			title: 'Random',
			'aria-label': 'random color',
		});
	};

	// challengerName (view/home.templ): who is waiting, or "Anonymous"
	const challengerName = (c, into) => {
		if (!c.n) {
			const anon = make('span', 'truncate text-sm font-semibold text-fg-muted');
			anon.textContent = 'Anonymous';
			into.appendChild(anon);
			return;
		}
		const badge = titleBadge(c.t, c.tn);
		if (badge) {
			into.appendChild(badge);
		}
		const name = make('span', 'truncate text-sm font-semibold text-fg');
		name.textContent = c.n;
		into.appendChild(name);
		if (c.rg) {
			const rating = make('span', 'rating-chip flex-none');
			rating.textContent = c.rg;
			into.appendChild(rating);
		}
	};

	// challengeRow (view/home.templ)
	const challengeRow = (c) => {
		const li = document.createElement('li');
		const a = make(
			'a',
			'flex items-center justify-between gap-3 rounded-md border border-line bg-panel ' +
			'px-3 py-2.5 no-underline transition duration-150 ease-snappy hover:-translate-y-px ' +
			'hover:border-accent hover:shadow-md',
			{href: '/' + c.r}
		);

		const left = make('span', 'flex min-w-0 items-center gap-2.5');
		left.appendChild(colorDot(c.c));
		const stack = make('span', 'flex min-w-0 flex-col leading-tight');

		const head = make('span', 'flex items-center gap-1.5');
		challengerName(c, head);
		if (c.rd) {
			// rated seeks are members-only; anons see the tag and log in to join
			const tag = make(
				'span',
				'inline-flex flex-none items-center gap-0.5 rounded-full border border-accent/40 ' +
				'px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wide text-accent'
			);
			tag.appendChild(svg(trophyMarkup));
			tag.appendChild(document.createTextNode('Rated'));
			head.appendChild(tag);
		}
		stack.appendChild(head);

		const meta = make('span', 'text-[11px] uppercase tracking-wide text-fg-subtle');
		let line = c.vn + ' · ' + c.sp;
		if (c.rt > 0) {
			line += ' · race to ' + c.rt;
		}
		meta.textContent = line;
		stack.appendChild(meta);

		left.appendChild(stack);
		a.appendChild(left);

		// a rated seek is joinable only by a member; an anonymous visitor is sent
		// to log in rather than shown a button that would refuse them
		const cta = c.rd && !self.name
			? make('span', 'btn btn-ghost flex-none px-3 py-1.5 text-xs')
			: make('span', 'btn btn-primary flex-none px-3 py-1.5 text-xs');
		cta.textContent = c.rd && !self.name ? 'Log in' : 'Join';
		a.appendChild(cta);

		li.appendChild(a);
		return li;
	};

	// ---- section renderers ----

	const fill = (list, items, build) => {
		if (!list) {
			return;
		}
		const frag = document.createDocumentFragment();
		items.forEach((it) => frag.appendChild(build(it)));
		list.replaceChildren(frag);
	};

	const applyStats = (s) => {
		text(el('home-stat-playing'), String(s.p || 0));
		text(el('home-stat-live'), String(s.l || 0));
		text(el('home-stat-total'), String(s.g || 0));
	};

	const applyChallenges = (ch) => {
		const items = (ch && ch.i) || [];
		fill(el('home-challenges-list'), items, challengeRow);
		show(el('home-challenges-list'), items.length > 0);
		show(el('home-challenges-empty'), items.length === 0);
	};

	// anonNote (view/home.go). The viewer-dependent half is decided here because
	// the count is broadcast and whether the reader is one of the anonymous
	// visitors is not.
	const anonNote = (anon) => {
		if (!anon || anon <= 0) {
			return '';
		}
		let s = anon === 1 ? '1 anonymous visitor' : anon + ' anonymous visitors';
		if (!self.name) {
			s += ' (including you)';
		}
		return s;
	};

	// The broadcast roster still contains the viewer and the people they follow,
	// because one payload serves everybody. Both come out here: a name in both
	// the Following section and "Online now" would read as two people, and the
	// viewer's own chip is the one row in the list with nothing to do.
	const visibleRoster = (online) => {
		const skip = new Set(following.map((m) => (m.n || '').toLowerCase()));
		if (self.name) {
			skip.add(self.name.toLowerCase());
		}
		return online.filter((m) => !skip.has((m.n || '').toLowerCase()));
	};

	// lastPlayers is held so a Following frame — which arrives on its own — can
	// re-filter the roster without waiting for the next broadcast.
	let lastPlayers = null;

	const applyPlayers = (pl) => {
		lastPlayers = pl;
		const roster = visibleRoster((pl && pl.o) || []);
		const arrivals = (pl && pl.n) || [];

		fill(el('home-online-list'), roster, rosterChip);
		show(el('home-online'), roster.length > 0);

		fill(el('home-arrivals-list'), arrivals, arrivalChip);
		show(el('home-arrivals'), arrivals.length > 0);

		const note = anonNote(pl && pl.a);
		text(el('home-anon'), note);
		show(el('home-anon'), note !== '');

		refreshCard();
	};

	const applyFollowing = (fl) => {
		following = (fl && fl.i) || [];
		fill(el('home-following-list'), following, rosterChip);
		show(el('home-following'), following.length > 0);
		// the roster below must drop these names, so re-render it from what the
		// last broadcast carried
		if (lastPlayers) {
			applyPlayers(lastPlayers);
		} else {
			refreshCard();
		}
	};

	// hasPlayers (view/home.go): an entirely empty panel renders nothing at all
	// rather than a stack of empty states — a quiet site should look quiet, not
	// broken.
	const refreshCard = () => {
		const any =
			(el('home-following-list') || {}).childElementCount > 0 ||
			(el('home-online-list') || {}).childElementCount > 0 ||
			(el('home-arrivals-list') || {}).childElementCount > 0;
		show(el('home-players'), any);
	};

	// ---- entry point, called by lio-tv.js for every 'hm' frame ----
	window.lioHomeActivity = {
		apply: (d) => {
			if (!d) {
				return;
			}
			// Self first: the roster and challenge renderers read it, and the hub
			// puts it in the same connect payload as the first of each.
			if (d.me) {
				self = {name: d.me.n || '', challenge: !!d.me.c};
			}
			if (d.st) {
				applyStats(d.st);
			}
			if (d.fl) {
				applyFollowing(d.fl);
			}
			if (d.pl) {
				applyPlayers(d.pl);
			}
			if (d.ch) {
				applyChallenges(d.ch);
			}
		},
	};
})();
