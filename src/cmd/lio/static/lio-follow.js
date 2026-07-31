// The follow surfaces (arch/FOLLOWING.md Phases 2 and 5): the profile's
// follower / following dialog, and the header popover listing the people this
// viewer follows.
//
// Both frames are server-rendered (view/follow.templ); this fills them. The
// rows live here rather than in templ because the dialog's list is paged: a Go
// renderer and a JavaScript renderer would both have to exist, and they would
// drift apart the first time a row grew a field — the same reason the
// notification panel renders in the client.
//
// One row renderer serves both surfaces, so a person cannot be described one
// way in the dialog and another in the popover. Each surface is guarded on its
// own element, so this file is inert wherever neither is present.
(function () {
  "use strict";

  const modal = document.getElementById("modalFollow");
  const panel = document.getElementById("followingPanel");
  if (!modal && !panel) return;

  // Whether this viewer may send a challenge at all. The server decides it —
  // a seated viewer may not, since a challenge issued from the board you are
  // sitting at would commit you to a second game — and hands the answer down on
  // the panel, because the client cannot work it out. The create-game dialog it
  // would open is mounted on every page now, so its presence no longer says
  // anything about whether a challenge is allowed.
  const canChallengeHere =
    !!panel && panel.dataset.canChallenge === "true";
  const owner = modal ? modal.dataset.followOwner || "" : "";
  // Who is reading. The dialog carries it; on a page that has only the header
  // popover, the presence of that popover is itself proof of a signed-in
  // viewer, so any non-empty marker will do.
  const viewer = modal ? modal.dataset.followViewer || "" : "me";
  // A row toggle changes how many accounts the *viewer* follows. That figure is
  // only on screen when the viewer is looking at their own profile, which is
  // the one case where the count beside the name has to move with the button.
  const viewerIsOwner =
    !!modal && !!viewer && viewer.toLowerCase() === owner.toLowerCase();

  // ------------------------------------------------------------------ render

  function empty(text) {
    const p = document.createElement("p");
    p.className = "follow-empty";
    p.textContent = text;
    return p;
  }

  // One row: who they are, whether they are here, and what the viewer can do
  // about them. The name is the link — the row is a person, and a profile is
  // where a person's information is.
  //
  // opts.toggle asks for the Follow/Following control. The profile's dialog is
  // a directory of people you may not know yet, so it offers one on every row;
  // the header popover is a list of people you have *already* chosen, where the
  // only thing that control can do is undo that choice by accident. It gets the
  // sword instead, which is what the panel is for.
  function rowEl(m, opts) {
    const row = document.createElement("li");
    row.className = "follow-row";

    const link = document.createElement("a");
    link.className = "follow-who";
    link.href = "/@/" + encodeURIComponent(m.name);

    if (m.online) {
      const dot = document.createElement("span");
      dot.className = "follow-dot";
      // The dot is decorative; the state is said in words below, so it is never
      // carried by color alone.
      dot.setAttribute("aria-hidden", "true");
      link.appendChild(dot);
    }
    if (m.title) {
      const t = document.createElement("span");
      t.className = "player-title";
      t.textContent = m.title;
      link.appendChild(t);
    }
    const name = document.createElement("span");
    name.className = "follow-name";
    name.textContent = m.name;
    link.appendChild(name);
    // What an online member is doing, in words rather than by the dot's colour
    // alone — and it says why the sword is missing rather than leaving that to
    // be inferred. The same vocabulary the home roster's chips use.
    if (m.online) {
      const tag = document.createElement("span");
      tag.className = "follow-tag";
      tag.textContent = m.playing ? "playing" : m.busy ? "waiting" : "online";
      link.appendChild(tag);
    }
    row.appendChild(link);

    // The sword, for somebody who could actually accept right now. Same rule as
    // the home roster's, and only where the row carries the flags to judge it:
    // the profile's list endpoints report Online but not Playing/Busy, so a
    // sword there could be offered for somebody already at a board.
    if (opts && opts.sword && canChallengeHere && m.online && !m.busy && !m.self) {
      row.appendChild(swordEl(m.name));
    }
    // No follow toggle for a signed-out reader (there is no account to follow
    // from) and none on the viewer's own row (the server refuses a self-follow,
    // and offering the control would be a promise it will not keep).
    if (opts && opts.toggle && viewer && !m.self) row.appendChild(toggleEl(m));
    return row;
  }

  // The crossed-swords control, matching the one the roster and the profile
  // render. It carries the same data attributes, so the create-game dialog's
  // delegated opener picks it up with no wiring of its own.
  function swordEl(name) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "follow-sword";
    btn.setAttribute("data-open-create-game", "");
    btn.setAttribute("data-challenge", name);
    btn.setAttribute("aria-label", "Challenge " + name);
    btn.title = "Challenge " + name;
    btn.appendChild(
      icon([
        "M14.5 17.5 3 6V3h3l11.5 11.5",
        "M13 19l6-6",
        "M16 16l4 4",
        "M19 21l2-2",
        "M9.5 17.5 21 6V3h-3L6.5 14.5",
        "M11 19l-6-6",
        "M8 16l-4 4",
        "M5 21l-2-2",
      ])
    );
    return btn;
  }

  // Builds one lucide-style stroke icon, node by node: the site's CSP is strict
  // and everything else here builds DOM the same way.
  function icon(paths) {
    const NS = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(NS, "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("fill", "none");
    svg.setAttribute("stroke", "currentColor");
    svg.setAttribute("stroke-width", "2");
    svg.setAttribute("stroke-linecap", "round");
    svg.setAttribute("stroke-linejoin", "round");
    svg.setAttribute("aria-hidden", "true");
    paths.forEach(function (d) {
      const p = document.createElementNS(NS, "path");
      p.setAttribute("d", d);
      svg.appendChild(p);
    });
    return svg;
  }

  function toggleEl(m) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "follow-mini";
    paintToggle(btn, !!m.following);
    btn.addEventListener("click", async function () {
      if (btn.classList.contains("is-busy")) return;
      const following = btn.classList.contains("is-following");
      btn.classList.add("is-busy");
      try {
        const res = await fetch("/api/follow/" + encodeURIComponent(m.name), {
          method: following ? "DELETE" : "POST",
          headers: { Accept: "application/json" },
        });
        if (res.ok) {
          const data = await res.json().catch(() => null);
          const now = data ? !!data.following : !following;
          // Keep the cached row in step, so switching tabs and coming back
          // does not show the state this click just left behind.
          m.following = now;
          paintToggle(btn, now);
          if (viewerIsOwner) bumpFollowing(now ? 1 : -1);
        } else {
          const err = await res.json().catch(() => null);
          btn.title = (err && err.error) || "Could not save that.";
        }
      } catch (e) {
        btn.title = "Network error — that did not save.";
      }
      btn.classList.remove("is-busy");
    });
    return btn;
  }

  // Both words are always present, stacked, so the button is sized by the
  // longer of them and a press never resizes the row.
  function paintToggle(btn, following) {
    btn.classList.toggle("is-following", following);
    btn.setAttribute("aria-pressed", following ? "true" : "false");
    if (!btn.firstChild) {
      ["off", "on"].forEach(function (s) {
        const span = document.createElement("span");
        span.className = "follow-line";
        span.dataset.state = s;
        span.textContent = s === "on" ? "Following" : "Follow";
        btn.appendChild(span);
      });
    }
  }

  // The owner's "following" figure, moved by a row toggle. Only ever called
  // when the viewer is the owner; see viewerIsOwner.
  function bumpFollowing(delta) {
    const el = document.querySelector("[data-following-count]");
    if (!el) return;
    const n = Math.max(0, (parseInt(el.textContent.replace(/[^0-9]/g, ""), 10) || 0) + delta);
    el.textContent = n.toLocaleString() + " following";
    el.disabled = n === 0;
  }

  // Paints rows into a container, or the given empty-state copy when there are
  // none. Shared by both surfaces; opts is passed through to rowEl.
  function paint(into, rows, emptyCopy, opts) {
    into.replaceChildren();
    if (!rows.length) {
      into.appendChild(empty(emptyCopy));
      return;
    }
    const ul = document.createElement("ul");
    ul.className = "follow-rows";
    rows.forEach(function (m) {
      ul.appendChild(rowEl(m, opts));
    });
    into.appendChild(ul);
  }

  // ------------------------------------------------- the profile's dialog

  if (modal) mountModal();

  function mountModal() {
  const list = modal.querySelector("[data-follow-list]");
  const moreBtn = modal.querySelector("[data-follow-more]");
  const closeBtn = modal.querySelector(".modal-close");
  const tabs = Array.from(modal.querySelectorAll("[data-follow-tab]"));

  // Per-tab state. Each tab remembers its own page and rows, so switching back
  // and forth costs nothing and does not lose a reader's place.
  const state = {
    followers: { page: 0, more: false, rows: [], loaded: false },
    following: { page: 0, more: false, rows: [], loaded: false },
  };
  let active = "followers";
  let loading = false;

  function render() {
    const s = state[active];
    paint(
      list,
      s.rows,
      active === "followers"
        ? "Nobody is following " + owner + " yet."
        : owner + " is not following anybody yet.",
      { toggle: true, sword: false }
    );
    if (moreBtn) moreBtn.hidden = !s.more;
  }

  // -------------------------------------------------------------------- data

  async function loadPage(tab) {
    if (loading) return;
    loading = true;
    const s = state[tab];
    try {
      const res = await fetch(
        "/api/follow/" + encodeURIComponent(owner) + "/" + tab +
          "?page=" + (s.page + 1),
        { headers: { Accept: "application/json" } }
      );
      if (res.ok) {
        const data = await res.json();
        // Appended, not replaced: "Load more" grows the list a reader is
        // already looking at.
        s.rows = s.rows.concat(data.members || []);
        s.page = data.page || s.page + 1;
        s.more = !!data.more;
        s.loaded = true;
      } else {
        s.loaded = true;
        list.replaceChildren(empty("Could not load that list."));
        loading = false;
        return;
      }
    } catch (e) {
      s.loaded = true;
      list.replaceChildren(empty("Could not load that list."));
      loading = false;
      return;
    }
    loading = false;
    if (tab === active) render();
  }

  // -------------------------------------------------------------------- open

  function show(tab) {
    active = tab;
    tabs.forEach(function (t) {
      const on = t.dataset.followTab === tab;
      t.classList.toggle("is-active", on);
      t.setAttribute("aria-selected", on ? "true" : "false");
    });
    const s = state[tab];
    if (!s.loaded) {
      list.replaceChildren(empty("Loading…"));
      if (moreBtn) moreBtn.hidden = true;
      loadPage(tab);
    } else {
      render();
    }
  }

  function open(tab) {
    modal.classList.add("open");
    show(tab);
  }

  function close() {
    modal.classList.remove("open");
  }

  // Delegated, so a count rendered into a region that is swapped later still
  // opens the dialog. The trigger names which list it stands for.
  document.addEventListener("click", function (e) {
    const trigger = e.target.closest("[data-follow-open]");
    if (!trigger || trigger.disabled) return;
    open(trigger.dataset.followOpen === "following" ? "following" : "followers");
  });

  tabs.forEach(function (t) {
    t.addEventListener("click", function () {
      show(t.dataset.followTab);
    });
  });
  if (moreBtn) {
    moreBtn.addEventListener("click", function () {
      loadPage(active);
    });
  }
  if (closeBtn) closeBtn.addEventListener("click", close);
  modal.addEventListener("click", function (e) {
    if (e.target === modal) close();
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape") close();
  });
  // Back/forward restores a page with its dialogs as they were left. A restored
  // open dialog is not a state anybody asked for.
  window.addEventListener("pageshow", function (e) {
    if (e.persisted) close();
  });

  // #followers / #following opens the dialog on that tab. It is what the
  // header popover's "All" link points at, so following it lands on the whole
  // list rather than merely on the profile that holds it — and it makes both
  // lists linkable, which a click-only dialog is not.
  //
  // hashchange as well as load: clicking "All" while already standing on your
  // own profile changes the fragment without navigating anywhere.
  function openFromHash() {
    const tab = (location.hash || "").replace("#", "").toLowerCase();
    if (tab === "followers" || tab === "following") open(tab);
  }
  openFromHash();
  window.addEventListener("hashchange", openFromHash);
  }

  // ------------------------------------------------- the header's popover

  if (panel) mountPanel();

  // The popover's rows: a sword, and no follow toggle. These are people the
  // viewer has already chosen, so the only thing a toggle could do here is undo
  // that choice by accident; the panel exists to start a game with one of them.
  const PANEL_OPTS = { toggle: false, sword: true };

  function mountPanel() {
    const into = document.getElementById("followingList");
    if (!into) return;

    // The panel is opened by navScript, like every other header popover, which
    // calls this hook so the fetch happens on the first open rather than on
    // every page load. Reopening repaints from the cached rows; the numbers it
    // shows are presence, which moves, so a reopen after a while refetches.
    let rows = null;
    let fetchedAt = 0;
    let loading = false;
    const STALE_MS = 15000;

    window.__followingPanelOpened = function () {
      if (rows && Date.now() - fetchedAt < STALE_MS) {
        paint(into, rows, "You are not following anybody yet.", PANEL_OPTS);
        return;
      }
      load();
    };

    async function load() {
      if (loading) return;
      loading = true;
      try {
        const res = await fetch("/api/follow/mine", {
          headers: { Accept: "application/json" },
        });
        if (res.ok) {
          const data = await res.json();
          rows = data.members || [];
          fetchedAt = Date.now();
          paint(into, rows, "You are not following anybody yet.", PANEL_OPTS);
          paintBadge(rows.filter(function (m) { return m.online; }).length);
        } else {
          into.replaceChildren(empty("Could not load your list."));
        }
      } catch (e) {
        into.replaceChildren(empty("Could not load your list."));
      }
      loading = false;
      into.dataset.loaded = "true";
    }

    // The badge the header painted at render time, corrected by what the fetch
    // actually found and, from then on, by the socket
    // (arch/HOME_ACTIVITY_STREAMING.md). Opening the panel is no longer the only
    // moment it can be made true.
    //
    // Exposed so the page's socket owner — whichever of lio.js / lio-tv.js /
    // lio-notify.js holds the connection — can push a new count in without
    // knowing anything about how the badge is drawn. A count that arrived
    // before this file parsed is picked up from the stash below.
    window.lioFollowBadge = { apply: paintBadge };
    if (typeof window.__lioFollowOnline === "number") {
      paintBadge(window.__lioFollowOnline);
    }

    function paintBadge(n) {
      const btn = document.getElementById("followingButton");
      if (!btn) return;
      const existing = btn.querySelector(".notify-dot");
      if (n <= 0) {
        if (existing) existing.remove();
        return;
      }
      const label =
        n === 1
          ? "1 player you follow is online"
          : n + " players you follow are online";
      if (existing) {
        existing.setAttribute("aria-label", label);
        return;
      }
      const dot = document.createElement("span");
      dot.className = "notify-dot is-online";
      dot.setAttribute("role", "status");
      dot.setAttribute("aria-label", label);
      btn.appendChild(dot);
    }
  }
})();
