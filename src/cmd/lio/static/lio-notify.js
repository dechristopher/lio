// The notification client: the badge on the header bell, the panel behind it,
// and the socket that keeps both current (arch/NOTIFICATIONS.md).
//
// It loads from the header on every page, for every signed-in visitor, and it
// does three jobs.
//
//  1. It paints the badge from the count the server sends. The count arrives on
//     every socket connect and with every new message, so nothing here polls.
//  2. It renders the panel. The list loads over HTTP on the first open, and a
//     message that arrives later is rendered by the same function, from the same
//     shape — the server sends one item shape on both paths.
//  3. It owns the page's socket when nothing else does. A page with a game or
//     the home TV stream already holds one, and notifications ride that; every
//     other page has none, so this opens /socket/me.
//
// The socket deliberately does not report to window.lioConn. That indicator
// describes a live component the reader is watching — a game, the TV grid — and
// lighting it on the about page because a background channel is open would say
// something the page is not doing.
(function () {
  "use strict";

  const bell = document.getElementById("notifyButton");
  const panel = document.getElementById("notifyPanel");
  const list = document.getElementById("notifyList");
  const readAll = document.getElementById("notifyReadAll");
  // Signed out: the header renders no bell, and nothing below has a target.
  if (!bell || !panel || !list) return;

  // The two counts the badge adds together. They are separate because they come
  // from different places and mean different things: unread is this account's
  // own messages, staff is the site-wide unread feedback a moderator can act on
  // (its read state belongs to the whole site, so it is derived rather than
  // stored per person).
  let unread = 0;
  let staff = 0;
  let loaded = false;
  let loading = false;

  // ---------------------------------------------------------------- badge

  // The dot is created and removed rather than shown and hidden, which is what
  // the server-rendered markup does (notifyDot renders nothing at zero). A page
  // that has been open for an hour and a page loaded a second ago then describe
  // "nothing new" the same way instead of drifting into two ways of saying it.
  function paintDot(host, n, label, className) {
    let dot = host.querySelector("." + className);
    if (n > 0) {
      if (!dot) {
        dot = document.createElement("span");
        dot.className = className;
        dot.setAttribute("role", "status");
        host.appendChild(dot);
      }
      dot.setAttribute("aria-label", label);
      dot.title = label;
    } else if (dot) {
      dot.remove();
    }
  }

  function badgeLabel(n) {
    return n === 1 ? "1 unread notification" : n + " unread notifications";
  }

  function staffLabel(n) {
    return n === 1 ? "1 unread feedback message" : n + " unread feedback messages";
  }

  function paintBadge() {
    const total = unread + staff;
    paintDot(bell, total, badgeLabel(total), "notify-dot");
    // The System link in the profile popover keeps its own dot: it points a
    // moderator at the inbox, which the bell does not.
    document.querySelectorAll("[data-unread-anchor]").forEach(function (el) {
      paintDot(el, staff, staffLabel(staff), "unread-dot");
    });
    if (readAll) readAll.classList.toggle("hidden", unread === 0);
  }

  // ---------------------------------------------------------------- rows

  // One glyph per kind. An unknown kind — a row written by a newer build and
  // read by a page the deploy has not replaced yet — gets the neutral glyph and
  // renders as a plain message. It must never throw, and it must never render
  // nothing: the body is always readable on its own.
  const glyphs = {
    mod_action: "\u{1F6E1}",
    milestone: "\u{1F4C8}",
    system: "⚙",
    staff: "\u{1F4AC}",
  };

  // Kinds whose mark is a drawn icon rather than an emoji, checked before the
  // glyph map above. An emoji renders as whatever the reader's operating system
  // decided it looks like; a stroked icon is the site's own, follows
  // currentColor, and matches the same glyph used on the buttons elsewhere on
  // the page. New kinds belong here rather than in glyphs.
  const svgGlyphs = {
    // lucide "users" — the glyph the home page's "vs Human" button carries
    // (iconUsers in view/components.templ). A follower is a person, and this is
    // already how the site draws people.
    follow: [
      "M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2",
      ["circle", { cx: 9, cy: 7, r: 4 }],
      "M23 21v-2a4 4 0 0 0-3-3.87",
      "M16 3.13a4 4 0 0 1 0 7.75",
    ],
  };

  function when(ms) {
    const secs = Math.max(0, Math.round((Date.now() - ms) / 1000));
    if (secs < 60) return "just now";
    const mins = Math.round(secs / 60);
    if (mins < 60) return mins + "m";
    const hours = Math.round(mins / 60);
    if (hours < 24) return hours + "h";
    return Math.round(hours / 24) + "d";
  }

  // A challenge is the only kind that expires, and the only one the panel
  // offers an action on.
  function isChallenge(item) {
    return item && item.k === "challenge";
  }

  // Live means "still worth offering an Accept for". The stamp is an upper
  // bound, not the authority: the room usually dies before it (its creator
  // leaves the waiting page), and accepting a dead one lands on the same
  // room-gone redirect a stale open challenge has always given. Read rows are
  // excluded because declining marks the row read — a refused challenge must
  // not keep offering the buttons.
  function isLive(item) {
    return isChallenge(item) && !item.r && item.x > Date.now();
  }

  // The countdown on a live challenge, as m:ss.
  function timeLeft(item) {
    const secs = Math.max(0, Math.round((item.x - Date.now()) / 1000));
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return m + ":" + (s < 10 ? "0" + s : s);
  }

  // The room a challenge points at. The link is this site's own path, built by
  // the server, so the id is what follows the single leading slash.
  function roomOf(item) {
    return item && item.l ? item.l.replace(/^\//, "") : "";
  }

  // Builds one row. Everything a message carries is set with textContent, never
  // innerHTML: the body is written by the server but an actor's name is a
  // person's own text, and a row is not a place to run it.
  function row(item) {
    // A live challenge is a block, not a link: it carries its own Accept and
    // Decline, and wrapping those in an anchor would make Decline navigate.
    const live = isLive(item);
    const el = document.createElement(item.l && !live ? "a" : "div");
    el.className = "notify-row kind-" + (item.k || "system") + (item.r ? "" : " is-unread");
    if (item.l && !live) el.href = item.l;
    if (item.id) el.dataset.id = item.id;

    const iconEl = document.createElement("span");
    iconEl.className = "notify-row-icon";
    iconEl.setAttribute("aria-hidden", "true");
    const shapes = svgGlyphs[item.k];
    if (shapes) {
      iconEl.appendChild(icon(shapes));
    } else {
      iconEl.textContent = glyphs[item.k] || "•";
    }

    const body = document.createElement("div");
    body.className = "notify-row-body";
    body.textContent = item.b || "";

    const meta = document.createElement("div");
    meta.className = "notify-row-meta";
    if (item.a) {
      const actor = document.createElement("span");
      actor.className = "notify-row-actor";
      actor.textContent = item.a;
      meta.appendChild(actor);
    }
    const time = document.createElement("time");
    // A live challenge counts down instead of counting up: what matters is how
    // long is left to answer, not how long ago it arrived.
    time.textContent = live ? timeLeft(item) + " left" : when(item.ts || Date.now());
    if (live) time.dataset.countdown = item.x;
    meta.appendChild(time);
    body.appendChild(meta);

    el.appendChild(iconEl);
    el.appendChild(body);
    if (live) el.appendChild(challengeActions(item));
    return el;
  }

  // Builds one lucide-style stroke icon. createElementNS rather than innerHTML:
  // everything else in this file builds DOM node by node, and the site's CSP is
  // strict enough that it is not worth introducing a second habit for a few
  // glyphs.
  //
  // A shape is either a path's "d" string or a [tag, attributes] pair, for the
  // primitives a path cannot express readably — a circle written as two arcs is
  // correct and unreadable, and these glyphs are copied from their templ twins
  // by hand.
  function icon(shapes) {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("fill", "none");
    svg.setAttribute("stroke", "currentColor");
    svg.setAttribute("stroke-width", "2.5");
    svg.setAttribute("stroke-linecap", "round");
    svg.setAttribute("stroke-linejoin", "round");
    svg.setAttribute("aria-hidden", "true");
    shapes.forEach(function (shape) {
      if (typeof shape === "string") {
        const p = document.createElementNS("http://www.w3.org/2000/svg", "path");
        p.setAttribute("d", shape);
        svg.appendChild(p);
        return;
      }
      const el = document.createElementNS("http://www.w3.org/2000/svg", shape[0]);
      Object.keys(shape[1]).forEach(function (k) {
        el.setAttribute(k, shape[1][k]);
      });
      svg.appendChild(el);
    });
    return svg;
  }

  const checkPath = ["M20 6 9 17l-5-5"];
  const crossPath = ["M18 6 6 18", "M6 6l12 12"];

  // Accept is a link to the room, because accepting *is* opening it: the room
  // page already handles seating the invited player, and it shows them the terms
  // before they commit. Decline needs a request — the challenger is sitting on
  // the waiting page, and closing the room is what tells them.
  //
  // Both are icons rather than words. The pair is a yes/no on one line, where a
  // tick and a cross are read faster than two labels of different lengths, and
  // the accessible name carries the word for anyone who needs it.
  function challengeActions(item) {
    const wrap = document.createElement("div");
    wrap.className = "notify-actions";

    const accept = document.createElement("a");
    accept.className = "notify-accept";
    accept.href = item.l;
    accept.setAttribute("aria-label", "Accept");
    accept.title = "Accept";
    accept.appendChild(icon(checkPath));

    const decline = document.createElement("button");
    decline.type = "button";
    decline.className = "notify-decline";
    decline.setAttribute("aria-label", "Decline");
    decline.title = "Decline";
    decline.appendChild(icon(crossPath));
    decline.addEventListener("click", function (e) {
      e.preventDefault();
      e.stopPropagation();
      // Mark it locally first: the row must stop offering an action the moment
      // it is clicked, whatever the request does. Declining a challenge whose
      // room has already gone is still a decline.
      item.r = true;
      post("/api/me/challenge/decline", { room: roomOf(item), id: item.id });
      removeToast(item.id);
      render(cached);
    });

    wrap.appendChild(accept);
    wrap.appendChild(decline);
    return wrap;
  }

  function empty(text) {
    const p = document.createElement("p");
    p.className = "notify-empty";
    p.textContent = text;
    return p;
  }

  // The last list the panel rendered, kept so reopening the panel costs no
  // request and so marking rows read can update what is on screen.
  let cached = [];

  function render(items) {
    cached = items || [];
    list.replaceChildren();
    // The derived staff row, for a moderator with unread feedback. It is not a
    // stored notification and has no id, so it is never marked read here — it
    // clears when the inbox on /system is worked.
    if (staff > 0) {
      list.appendChild(row({
        k: "staff",
        b: staffLabel(staff),
        l: "/system",
        ts: Date.now(),
        r: true,
      }));
    }
    if (!cached.length) {
      if (staff === 0) list.appendChild(empty("Nothing yet."));
      return;
    }
    // Live challenges first. They expire, and everything below them does not —
    // a challenge with forty seconds left must not sit under a week of rating
    // records. The rest keep the newest-first order the server sent.
    const live = cached.filter(isLive);
    const rest = cached.filter(function (i) { return !isLive(i); });
    live.concat(rest).forEach(function (item) { list.appendChild(row(item)); });
    syncCountdowns();
  }

  // The countdown on any live challenge, repainted once a second while
  // something is showing one. The ticker only exists while it has work: a page
  // with no pending challenge runs no timer at all.
  let ticker = null;
  function syncCountdowns() {
    const cells = document.querySelectorAll("[data-countdown]");
    if (!cells.length) {
      if (ticker) { clearInterval(ticker); ticker = null; }
      return;
    }
    if (!ticker) ticker = setInterval(syncCountdowns, 1000);
    let expired = false;
    cells.forEach(function (cell) {
      const left = Number(cell.dataset.countdown) - Date.now();
      if (left <= 0) {
        expired = true;
        return;
      }
      const secs = Math.round(left / 1000);
      const m = Math.floor(secs / 60);
      const s = secs % 60;
      cell.textContent = m + ":" + (s < 10 ? "0" + s : s) + " left";
    });
    // A challenge that ran out stops being actionable, so the rows are rebuilt
    // rather than left showing buttons that would now fail.
    if (expired) {
      dismissToastsFor(function (item) { return !isLive(item); });
      render(cached);
    }
  }

  // ---------------------------------------------------------------- data

  async function load() {
    if (loading) return;
    loading = true;
    try {
      const res = await fetch("/api/me/notifications", {
        headers: { Accept: "application/json" },
      });
      if (!res.ok) {
        // Leave whatever is on screen. A failed read must not replace a correct
        // list with an empty one.
        if (!loaded) render([]);
        return;
      }
      const data = await res.json();
      if (data && typeof data.unread === "number") unread = data.unread;
      render(data && data.items ? data.items : []);
      loaded = true;
      paintBadge();
    } catch (e) {
      if (!loaded) render([]);
    } finally {
      loading = false;
    }
  }

  async function post(url, body) {
    try {
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: body ? JSON.stringify(body) : "{}",
      });
      if (!res.ok) return;
      const data = await res.json();
      if (data && typeof data.unread === "number") {
        unread = data.unread;
        paintBadge();
      }
    } catch (e) { /* the badge is corrected on the next connect */ }
  }

  // ---------------------------------------------------------------- panel

  // navScript owns opening and closing the panel and calls this on every open.
  // The dot means "something is new", so seeing the panel is what clears it. The
  // rows keep their own unread mark until they are read, which is the different
  // question of whether the reader acted on any of them.
  window.__notifyPanelOpened = function () {
    if (!loaded) {
      load();
    } else {
      // repaint the relative times, which have aged since the last render
      render(cached);
    }
    if (unread > 0) markAllRead();
  };

  function markAllRead() {
    // A live challenge is not finished by being looked at, so the server leaves
    // it unread and the badge keeps reporting it. Predicting that here keeps the
    // badge from flashing to zero and back when the response lands.
    const pending = cached.filter(isLive).length;
    // Paint first: the reader has seen the panel, and the badge must not sit
    // there while a request is in flight. The response corrects the number.
    unread = pending;
    paintBadge();
    // The rows on screen keep their highlight for this viewing. Marking them
    // read is about the badge, and stripping the marks in the same instant the
    // panel opened would take away the one thing the reader came to see: which
    // of these are the new ones. The cached copies are updated, so the next time
    // the panel renders they are read — except a live challenge, which must keep
    // offering its buttons until it is answered.
    cached.forEach(function (item) {
      if (!isLive(item)) item.r = true;
    });
    post("/api/me/notifications/read-all");
  }

  if (readAll) {
    readAll.addEventListener("click", function (e) {
      e.stopPropagation();
      markAllRead();
    });
  }

  // A click on a row follows its link. Mark that one read on the way out so it
  // is not still tinted when the reader comes back.
  list.addEventListener("click", function (e) {
    const el = e.target.closest(".notify-row[data-id]");
    if (!el || !el.classList.contains("is-unread")) return;
    el.classList.remove("is-unread");
    post("/api/me/notifications/read", { id: Number(el.dataset.id) });
  });

  // ---------------------------------------------------------------- toast

  // A toast is for the arrivals that cannot wait for somebody to open the bell.
  // Today that is a challenge, which expires; every other kind can be read
  // whenever its reader gets to it, and interrupting them for it would train
  // them to ignore the thing that matters.
  //
  // The host is fixed, so nothing it does can move the page. It is created on
  // first use rather than rendered into every page's markup — most pages never
  // show one.
  let toastHost = null;
  const toasts = new Map(); // notification id -> element

  function ensureToastHost() {
    if (!toastHost) {
      toastHost = document.createElement("div");
      toastHost.className = "notify-toasts";
      toastHost.setAttribute("role", "status");
      toastHost.setAttribute("aria-live", "polite");
      document.body.appendChild(toastHost);
    }
    return toastHost;
  }

  // A game in progress is not a place to be interrupted. A page that owns the
  // room socket is a board the reader is sitting at, so the arrival goes to the
  // badge only — no card over the position, no sound during a game.
  //
  // Coarser than "while a clock runs", deliberately: the clock's state lives in
  // lio-game.js, and reaching into it from here would couple this file to the
  // game's internals to suppress a card during the few pre-game seconds when it
  // would be just as unwelcome.
  function inGame() {
    return window.lioSocketOwner === "room";
  }

  function showToast(item) {
    if (!isLive(item) || inGame() || toasts.has(item.id)) return;
    const host = ensureToastHost();

    // The card is a two-part surface, like the roster pill: the message on the
    // left, dismiss as its own full-height strip on the right. It was an X
    // floating over the corner of the message, which overlapped the row inside
    // it and read as part of the message rather than as a control.
    const card = document.createElement("div");
    card.className = "notify-toast";
    card.appendChild(row(item));

    const dismiss = document.createElement("button");
    dismiss.type = "button";
    dismiss.className = "notify-toast-close";
    dismiss.setAttribute("aria-label", "Dismiss");
    dismiss.title = "Dismiss";
    dismiss.appendChild(icon(crossPath));
    // Dismissing the card is not declining. It says "not now, stop covering my
    // page", and the challenge stays in the bell until it is answered or runs
    // out. Conflating the two would decline by accident — which is exactly why
    // this control must not look like the cross inside the message.
    dismiss.addEventListener("click", function () { removeToast(item.id); });
    card.appendChild(dismiss);

    host.appendChild(card);
    toasts.set(item.id, card);
    ping();
  }

  function removeToast(id) {
    const el = toasts.get(id);
    if (el) el.remove();
    toasts.delete(id);
  }

  // Drops every toast whose item now satisfies gone — used when a challenge
  // expires or is answered, so a dead card never sits there offering an Accept.
  function dismissToastsFor(gone) {
    cached.forEach(function (item) {
      if (toasts.has(item.id) && gone(item)) removeToast(item.id);
    });
  }

  // A plain Audio element, not Howler: this file loads on every page, and
  // Howler only ships with the game bundles — the home page has none. Playback
  // before any user gesture is refused by the browser's autoplay policy, which
  // is why the rejection is swallowed. Nothing here depends on the sound.
  function ping() {
    if (inGame()) return;
    try {
      const a = new Audio("/res/sfx/confirmation.ogg");
      a.volume = 0.5;
      const played = a.play();
      if (played && played.catch) played.catch(function () { /* not unlocked yet */ });
    } catch (e) { /* no audio on this page */ }
  }

  // ---------------------------------------------------------------- frames

  // The one entry point for both socket owners (lio.js, lio-tv.js) and for this
  // file's own socket. The count is taken from the frame and never counted up
  // locally: a client that added one for each arrival would drift the first time
  // a frame was dropped or repeated, and this badge has to stay correct on a
  // page nobody reloads for hours.
  // A frame carries only the counts it knows. A staff frame goes to every
  // moderator at once and cannot know any one of their personal counts; a
  // personal frame says nothing about the shared feedback backlog. So each count
  // is replaced only when the frame actually carries it — an absent field is
  // "unchanged", not zero.
  function apply(d) {
    if (!d) return;
    if (typeof d.n === "number") unread = d.n;
    if (typeof d.s === "number") staff = d.s;
    paintBadge();
    if (d.i) {
      // A new message invalidates the list the panel is holding. Reloading it
      // now would fetch on every arrival for a panel nobody has opened, so mark
      // it stale and let the next open pay for it.
      loaded = false;
      // A challenge cannot wait for somebody to open the bell — it expires. The
      // item is put in the cache first so the toast and the panel render the
      // same row, and so declining from the toast updates both.
      if (isLive(d.i)) {
        cached = [d.i].concat(cached.filter(function (i) { return i.id !== d.i.id; }));
        showToast(d.i);
      }
    }
    // Unless the panel is open right now, in which case the reader is looking
    // at the list this frame just changed.
    if (!panel.classList.contains("hidden")) {
      if (!loaded) load(); else render(cached);
    }
  }

  // ---------------------------------------------------------------- socket

  const reconnectBaseMs = 1000;
  const reconnectCapMs = 30000;
  let sock = null;
  let attempts = 0;
  let stopped = false;

  function connect() {
    if (sock || stopped) return;
    window.lioSocketOwner = "me";
    sock = new WebSocket(location.origin.replace(/^http/, "ws") + "/socket/me");
    sock.onopen = function () { attempts = 0; };
    sock.onmessage = function (evt) {
      let msg;
      try { msg = JSON.parse(evt.data); } catch (e) { return; }
      if (msg.t === "nt" && msg.d) apply(msg.d);
      // The version hello rides every socket. On a page whose only socket is
      // this one, it is the only thing that would ever notice a deploy.
      if (msg.t === "si" && msg.d && msg.d.v && window.lioUpdateNotice) {
        window.lioUpdateNotice(msg.d.v);
      }
    };
    sock.onclose = function () {
      sock = null;
      if (stopped) return;
      attempts++;
      const ceil = Math.min(reconnectCapMs, reconnectBaseMs * Math.pow(2, attempts));
      setTimeout(connect, Math.random() * ceil);
    };
  }

  // Called by lio.js when a game page gives up its socket for good and the
  // reader stays to review the game.
  function takeover() {
    stopped = false;
    connect();
  }

  window.lioNotify = { apply: apply, takeover: takeover };

  window.addEventListener("pagehide", function () {
    stopped = true;
    if (sock) {
      try { sock.close(); } catch (e) { /* already closing */ }
    }
  });

  // Decided on load, not at script time. lio-game.js opens the room socket from
  // its own load handler, which it registered before this deferred file ran, so
  // by the time this runs the page's real owner has claimed it. Claiming any
  // earlier would open a second socket on every game page.
  window.addEventListener("load", function () {
    if (!window.lioSocketOwner) connect();
  });
})();
