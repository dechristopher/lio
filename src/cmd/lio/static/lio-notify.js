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

  // One glyph per kind, every one a stroked SVG. No emoji: an emoji renders as
  // whatever the reader's operating system decided it looks like, at a weight
  // nothing else on the page shares, while a drawn glyph follows currentColor
  // and sits in the row like the rest of the site's chrome.
  //
  // Each is the glyph the site already uses for that idea, so a row's mark and
  // the surface it points at agree:
  //
  //   mod_action  lucide "shield"    the moderation surfaces
  //   milestone   iconChart          the profile's stats (view/components.templ)
  //   system      iconGear           the preferences and site controls
  //   staff       iconMessage        the feedback inbox (view/feedback.templ)
  //   challenge   iconSwords         the challenge control everywhere else
  //   follow      iconUsers          the "vs Human" glyph; a follower is a person
  //   announce    lucide "megaphone" a broadcast: the site talking to everybody
  //
  // New kinds belong here. Copy the glyph from its templ twin rather than
  // drawing a second version of it.
  const glyphs = {
    mod_action: ["M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"],
    milestone: ["M18 20V10", "M12 20V4", "M6 20v-6"],
    system: [
      ["circle", { cx: 12, cy: 12, r: 3 }],
      "M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z",
    ],
    staff: ["M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"],
    challenge: [
      "M14.5 17.5 3 6V3h3l11.5 11.5",
      "M13 19l6-6",
      "M16 16l4 4",
      "M19 21l2-2",
      "M9.5 17.5 21 6V3h-3L6.5 14.5",
      "M11 19l-6-6",
      "M8 16l-4 4",
      "M5 21l-2-2",
    ],
    follow: [
      "M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2",
      ["circle", { cx: 9, cy: 7, r: 4 }],
      "M23 21v-2a4 4 0 0 0-3-3.87",
      "M16 3.13a4 4 0 0 1 0 7.75",
    ],
    announce: ["m3 11 18-5v12L3 14v-3z", "M11.6 16.8a3 3 0 1 1-5.8-1.6"],
  };

  // lucide "info", for a kind this build does not know — a row written by a
  // newer one during a deploy. The row must never throw and must never render
  // nothing: the body is always readable on its own, and this says only "a
  // message", which is exactly as much as an older page can honestly say.
  const unknownGlyph = [
    ["circle", { cx: 12, cy: 12, r: 10 }],
    "M12 16v-4",
    "M12 8h.01",
  ];

  // A row's identity in the panel.
  //
  // The list is two stores merged: this account's notifications, and the
  // broadcasts every account reads. They have separate id sequences, so a
  // notification and a broadcast can carry the same number and are not the same
  // row — a lookup on the bare id would find whichever came first, mark the
  // wrong one read, and answer the wrong question. Everything that addresses a
  // row goes through this: the DOM key, the toast map, the busy sets, and every
  // lookup in the cache.
  function key(item) {
    if (!item || !item.id) return "";
    return (item.bc ? "b" : "n") + item.id;
  }

  function find(k) {
    return cached.find(function (i) { return key(i) === k; });
  }

  // A message that demands an answer. It is not finished by being looked at:
  // read-all leaves it, clicking it does not clear it, and only one of its own
  // options does. The generalization of the rule a live challenge was the first
  // instance of.
  function asks(item) {
    return !!(item && item.c && item.c.length);
  }

  function needsAnswer(item) {
    return asks(item) && !item.an;
  }

  // Everything the reader still has to act on: a challenge that expires, and a
  // question that is waiting. These float to the top of the panel, survive
  // read-all, and are the only arrivals that raise a toast.
  function needsAction(item) {
    return isLive(item) || needsAnswer(item);
  }

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

  // A follow row carries the reader's own edge back to their new follower, so it
  // offers the same toggle every other follow control on the site does — the
  // point of the message is that somebody is worth following back, and making
  // the reader open a profile to do it is a trip for one click.
  //
  // An actor with no name is one who has since deleted their account. The
  // message still reads on its own; there is simply nobody left to follow.
  function canFollowBack(item) {
    return !!(item && item.k === "follow" && item.a);
  }

  // Builds one row. Everything a message carries is set with textContent, never
  // innerHTML: the body is written by the server but an actor's name is a
  // person's own text, and a row is not a place to run it.
  function row(item) {
    // A row that carries a control is a block, not a link. A live challenge has
    // Accept and Decline; a question has its options; a follow has the
    // follow-back toggle. Either way a button inside an anchor is not something
    // the HTML allows, and every press would navigate.
    const live = isLive(item);
    const question = needsAnswer(item);
    const toggle = canFollowBack(item);
    const control = question || toggle;
    // Whether the row has a destination of its own, separate from whether the
    // row element is the thing that carries it.
    const linked = !!item.l && !live;
    const el = document.createElement(linked && !control ? "a" : "div");
    el.className = "notify-row kind-" + (item.k || "system") + (item.r ? "" : " is-unread");
    if (linked && !control) el.href = item.l;
    if (item.id) {
      el.dataset.key = key(item);
      el.dataset.id = item.id;
      if (item.bc) el.dataset.bc = "1";
    }

    const iconEl = document.createElement("span");
    iconEl.className = "notify-row-icon";
    iconEl.setAttribute("aria-hidden", "true");
    iconEl.appendChild(icon(glyphs[item.k] || unknownGlyph));

    const body = document.createElement("div");
    body.className = "notify-row-body";
    const named = writeBody(item, body);

    const meta = document.createElement("div");
    meta.className = "notify-row-meta";
    // Only when the sentence has not already said who this is about. A row
    // that names its subject and then repeats it underneath reads as two
    // different people at a glance.
    if (item.a && !named) {
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

    // A row with both a destination and a control is two surfaces side by side,
    // like the roster pill: the message links to the person, the button acts on
    // them. Two targets, separately clickable, that read as one object — because
    // that is what they are.
    const linkEl = linked && control ? document.createElement("a") : el;
    if (linkEl !== el) {
      linkEl.className = "notify-row-link";
      linkEl.href = item.l;
    }
    linkEl.appendChild(iconEl);
    linkEl.appendChild(body);
    if (linkEl !== el) el.appendChild(linkEl);
    if (live) el.appendChild(challengeActions(item));
    if (question) el.appendChild(choicesEl(item));
    if (item.an) el.appendChild(answeredEl(item));
    if (toggle) el.appendChild(followBackEl(item));
    return el;
  }

  // The options on a question, as full buttons rather than the tick-and-cross a
  // challenge uses. A challenge is always the same yes/no, so a matched icon
  // pair reads faster than words; these carry whatever the sender wrote, and
  // the label is the whole message of the control.
  //
  // Nothing here closes over the item or the button, for the reason followBackEl
  // records: the panel rebuilds its rows whenever a frame arrives, so a captured
  // reference can outlive the row it was made for. The row key and the chosen
  // label are both plain strings, and everything is looked up from them.
  function choicesEl(item) {
    const wrap = document.createElement("div");
    wrap.className = "notify-choices";
    const k = key(item);
    const busy = answerBusy.has(k);
    item.c.forEach(function (choice) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "notify-choice" + (busy ? " is-busy" : "");
      btn.textContent = choice;
      // A refusal is kept on the item so it survives the rebuild, like the
      // follow-back's. It says why the press did nothing.
      if (item.anError) btn.title = item.anError;
      btn.addEventListener("click", function (e) {
        e.preventDefault();
        e.stopPropagation();
        answer(k, choice);
      });
      wrap.appendChild(btn);
    });
    return wrap;
  }

  // What the reader answered, once they have. It replaces the buttons rather
  // than sitting beside them: the question is closed, and the row still has to
  // say which way — otherwise somebody who answered an offer in one tab cannot
  // tell in another whether they did.
  function answeredEl(item) {
    const p = document.createElement("p");
    p.className = "notify-answered";
    p.appendChild(document.createTextNode("You answered "));
    const choice = document.createElement("span");
    choice.className = "notify-answered-choice";
    choice.textContent = item.an;
    p.appendChild(choice);
    return p;
  }

  // Row keys with an answer in flight. Held here rather than as a class on the
  // button, for the same reason followBusy is: a rebuilt button would come back
  // pressable in the middle of its own request.
  const answerBusy = new Set();

  // Records one answer. The first answer stands on the server, so this never
  // offers a way to change it — a failure repaints the buttons rather than
  // pretending the answer landed.
  async function answer(k, choice) {
    if (answerBusy.has(k)) return;
    const item = find(k);
    if (!item) return;
    answerBusy.add(k);
    render(cached);

    try {
      const res = await fetch("/api/me/notifications/answer", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ id: item.id, bc: !!item.bc, choice: choice }),
      });
      const data = await res.json().catch(function () { return null; });
      if (res.ok) {
        item.an = choice;
        item.r = true;
        item.anError = "";
        if (data && typeof data.unread === "number") unread = data.unread;
      } else {
        item.anError = (data && data.error) || "Could not record that.";
      }
    } catch (e) {
      item.anError = "Network error — that did not save.";
    }
    answerBusy.delete(k);
    // An answered question stops needing the reader, so its card goes with it.
    removeToast(k);
    paintBadge();
    render(cached);
  }

  // Fills a row's body and reports whether the sentence names the actor.
  //
  // A follow row is the one kind whose subject belongs in the sentence, and it
  // is composed here rather than stored, because the name is the one part of a
  // notification that is resolved fresh on every read (the wire's `a`). The
  // stored body deliberately has no subject in it — see notifyFollowed in
  // www/handlers/api/follow: a name written into the row at the time of the
  // follow is a name that goes stale the moment its owner is renamed. So the
  // stored sentence stays the fallback, and it is what a row whose follower has
  // since deleted their account still reads correctly as.
  function writeBody(item, body) {
    if (item.k === "follow" && item.a) {
      const name = document.createElement("b");
      name.className = "notify-name";
      name.textContent = item.a;
      body.appendChild(name);
      body.appendChild(document.createTextNode(" is now following you"));
      return true;
    }
    body.textContent = item.b || "";
    return false;
  }

  // The follow-back toggle: the same two-state control the follow lists render
  // (follow-mini in lio-follow.js), with the first label saying which of the two
  // this is. Both labels are always in the DOM, stacked, so the button is sized
  // by the longer of them and pressing it never resizes the row.
  //
  // It toggles rather than disappearing once pressed, which is why the server
  // sends the relationship and not "offer a follow-back": a control that
  // vanishes under the reader leaves them unable to undo a misclick, and every
  // other follow button on the site works this way.
  //
  // Nothing here closes over the button or the item. The panel rebuilds its rows
  // from the cache whenever a frame arrives while it is open, and reloading the
  // list replaces the item objects wholesale — so a captured reference can
  // outlive the row it was made for. Pressing this used to write the follow and
  // then paint a button that was no longer on the page: the edge changed and the
  // label did not. Everything below goes through the row id instead.
  function followBackEl(item) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "follow-mini";
    ["off", "on"].forEach(function (state) {
      const span = document.createElement("span");
      span.className = "follow-line";
      span.dataset.state = state;
      span.textContent = state === "on" ? "Following" : "Follow back";
      btn.appendChild(span);
    });
    paintButton(btn, item);
    const k = key(item);
    btn.addEventListener("click", function () { toggleFollow(k); });
    return btn;
  }

  // Row keys with a follow write in flight. Held here rather than as a class on
  // the button for the same reason: a rebuilt button would come back pressable
  // in the middle of its own request.
  const followBusy = new Set();

  function paintButton(btn, item) {
    const following = !!item.fw;
    btn.classList.toggle("is-following", following);
    btn.classList.toggle("is-busy", followBusy.has(key(item)));
    btn.setAttribute("aria-pressed", following ? "true" : "false");
    // A refusal is kept on the item, not on the element, so it survives the
    // rebuild too. A closed account is the one worth reading: it says why the
    // button did nothing, and it is not the reader's doing.
    if (item.fwError) btn.title = item.fwError;
  }

  // Repaints the row's button from the row's current item, both found by key.
  function syncFollow(k) {
    const item = find(k);
    const btn = list.querySelector('.notify-row[data-key="' + k + '"] > .follow-mini');
    if (item && btn) paintButton(btn, item);
  }

  async function toggleFollow(k) {
    if (followBusy.has(k)) return;
    const item = find(k);
    if (!item || !item.a) return;
    followBusy.add(k);
    syncFollow(k);

    const following = !!item.fw;
    try {
      const res = await fetch("/api/follow/" + encodeURIComponent(item.a), {
        method: following ? "DELETE" : "POST",
        headers: { Accept: "application/json" },
      });
      const data = await res.json().catch(function () { return null; });
      if (res.ok) {
        // The server answers with the state the edge landed in, which is the
        // one to trust: both writes are idempotent, so a button that started on
        // the wrong label still ends on the right one.
        item.fw = data ? !!data.following : !following;
        item.fwError = "";
      } else {
        item.fwError = (data && data.error) || "Could not save that.";
      }
    } catch (e) {
      item.fwError = "Network error — that did not save.";
    }
    followBusy.delete(k);
    syncFollow(k);
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
      removeToast(key(item));
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
        // the People page, where the inbox is — not the console's front page.
        // The row exists to take a moderator to the unread feedback, and the
        // overview does not have it.
        l: "/system/people",
        ts: Date.now(),
        r: true,
      }));
    }
    if (!cached.length) {
      if (staff === 0) list.appendChild(empty("Nothing yet."));
      return;
    }
    // Anything still waiting on the reader goes first: a challenge that expires,
    // a question that has not been answered. Neither can be dealt with by
    // scrolling past it, and a challenge with forty seconds left must not sit
    // under a week of rating records. The rest keep the newest-first order the
    // server sent, across both stores.
    const waiting = cached.filter(needsAction);
    const rest = cached.filter(function (i) { return !needsAction(i); });
    waiting.concat(rest).forEach(function (item) { list.appendChild(row(item)); });
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
      dismissToastsFor(function (item) { return !needsAction(item); });
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
    // A row that is waiting on the reader — a live challenge, an unanswered
    // question — is not finished by being looked at, so the server leaves it
    // unread and the badge keeps reporting it. Predicting that here keeps the
    // badge from flashing to zero and back when the response lands.
    const pending = cached.filter(needsAction).length;
    // Paint first: the reader has seen the panel, and the badge must not sit
    // there while a request is in flight. The response corrects the number.
    unread = pending;
    paintBadge();
    // The rows on screen keep their highlight for this viewing. Marking them
    // read is about the badge, and stripping the marks in the same instant the
    // panel opened would take away the one thing the reader came to see: which
    // of these are the new ones. The cached copies are updated, so the next time
    // the panel renders they are read — except a row that is still waiting,
    // which must keep offering its buttons until it is answered.
    cached.forEach(function (item) {
      if (!needsAction(item)) item.r = true;
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
    const el = e.target.closest(".notify-row[data-key]");
    if (!el || !el.classList.contains("is-unread")) return;
    const k = el.dataset.key;
    const item = find(k);
    // Clicking a question is not answering it. The server refuses this write
    // for exactly that reason; stopping here as well keeps the row on screen
    // from claiming otherwise.
    if (item && needsAnswer(item)) return;
    el.classList.remove("is-unread");
    // The cached row too, or the next re-render brings the mark back. Most rows
    // navigate away on this click and never see it; a row with a control of its
    // own — the follow-back toggle — is pressed and stayed on.
    if (item) item.r = true;
    // bc routes the write: a notification is stamped read, while a broadcast is
    // one row shared by everybody and moves this account's watermark instead.
    post("/api/me/notifications/read", {
      id: Number(el.dataset.id),
      bc: el.dataset.bc === "1",
    });
  });

  // ---------------------------------------------------------------- toast

  // A toast is for the arrivals that cannot wait for somebody to open the bell:
  // a challenge, which expires, and a question, which stays until it is
  // answered. Every other kind can be read whenever its reader gets to it, and
  // interrupting them for it would train them to ignore the thing that matters.
  //
  // That line is why an ordinary broadcast raises no card. It reaches everybody
  // at once, which makes it the easiest thing on the site to over-use; it goes
  // to the badge and waits, like any other message somebody can read later.
  //
  // The host is fixed, so nothing it does can move the page. It is created on
  // first use rather than rendered into every page's markup — most pages never
  // show one.
  let toastHost = null;
  const toasts = new Map(); // row key -> element

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
    const k = key(item);
    if (!needsAction(item) || inGame() || toasts.has(k)) return;
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
    // Dismissing the card is not declining, and it is not answering. It says
    // "not now, stop covering my page", and the row stays in the bell until it
    // is dealt with. Conflating the two would decline by accident — which is
    // exactly why this control must not look like the cross inside the message,
    // and why it stays on a question whose options sit alongside it.
    dismiss.addEventListener("click", function () { removeToast(k); });
    card.appendChild(dismiss);

    host.appendChild(card);
    toasts.set(k, card);
    ping();
  }

  function removeToast(k) {
    const el = toasts.get(k);
    if (el) el.remove();
    toasts.delete(k);
  }

  // Drops every toast whose item now satisfies gone — used when a challenge
  // expires or is answered, so a dead card never sits there offering an Accept.
  function dismissToastsFor(gone) {
    cached.forEach(function (item) {
      const k = key(item);
      if (toasts.has(k) && gone(item)) removeToast(k);
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
      const k = key(d.i);
      const known = cached.some(function (i) { return key(i) === k; });
      // A broadcast frame carries no count, and cannot: one frame goes to every
      // account on the site and their totals all differ. This is the only place
      // the client is allowed to move the number itself, and here it is exact
      // rather than a guess — a message that was just created is unread for
      // every account that existed before it, without exception. Keying the
      // bump on the row stops a repeated frame counting twice, and the next
      // socket connect replaces the total outright either way.
      if (d.i.bc && !known) {
        unread += 1;
        paintBadge();
      }
      // A row that is waiting on the reader cannot wait for somebody to open
      // the bell. The item is put in the cache first so the toast and the panel
      // render the same row, and so answering from the toast updates both.
      if (needsAction(d.i)) {
        cached = [d.i].concat(cached.filter(function (i) { return key(i) !== k; }));
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
    sock.onopen = function () {
      attempts = 0;
      // On every page that is neither a room nor the home page, this is the
      // only socket — so it is the one the username hover card watches rooms
      // over (arch/PLAYER_CARD.md). The card holds no connection of its own.
      if (window.lioCard) {
        window.lioCard.wire(function (obj) {
          try {
            if (sock && sock.readyState === WebSocket.OPEN) {
              sock.send(JSON.stringify(obj));
              return true;
            }
          } catch (e) { /* ignore */ }
          return false;
        });
      }
    };
    sock.onmessage = function (evt) {
      let msg;
      try { msg = JSON.parse(evt.data); } catch (e) { return; }
      if (msg.t === "nt" && msg.d) apply(msg.d);
      // the reconnect bar (arch/ONE_GAME_AT_A_TIME.md). On a page that is
      // neither a room nor the home page, this socket is the only one, so it is
      // the only way the bar ever learns a game ended.
      if (msg.t === "lg" && window.lioLiveGame) {
        window.lioLiveGame.apply(msg.d || {});
      }
      // the following badge's count. This socket is the only one on a page
      // that is neither a room nor the home page, so for a reader on their
      // profile or the about page it is the only way the badge ever moves.
      if (msg.t === "fo" && msg.d) {
        window.__lioFollowOnline = msg.d.o;
        if (window.lioFollowBadge) window.lioFollowBadge.apply(msg.d.o);
      }
      // The version hello rides every socket. On a page whose only socket is
      // this one, it is the only thing that would ever notice a deploy.
      if (msg.t === "si" && msg.d && msg.d.v && window.lioUpdateNotice) {
        window.lioUpdateNotice(msg.d.v);
      }
      // a room the hover card asked to watch, addressed to this connection
      if (msg.t === "wg" && window.lioCard) {
        window.lioCard.live(msg.d || {});
      }
    };
    sock.onclose = function () {
      sock = null;
      if (window.lioCard) window.lioCard.wire(null);
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
