// The follower / following lists (arch/FOLLOWING.md Phase 2).
//
// The dialog's frame is server-rendered (view/follow.templ); this fills it. The
// rows live here rather than in templ because the list is paged: a Go renderer
// and a JavaScript renderer would both have to exist, and they would drift
// apart the first time a row grew a field — the same reason the notification
// panel renders in the client.
//
// Everything is scoped to the dialog's own elements and every one of them is
// optional, so this file is inert on a page that has no follow lists.
(function () {
  "use strict";

  const modal = document.getElementById("modalFollow");
  if (!modal) return;

  const list = modal.querySelector("[data-follow-list]");
  const moreBtn = modal.querySelector("[data-follow-more]");
  const closeBtn = modal.querySelector(".modal-close");
  const tabs = Array.from(modal.querySelectorAll("[data-follow-tab]"));
  const owner = modal.dataset.followOwner || "";
  const viewer = modal.dataset.followViewer || "";
  // A row toggle changes how many accounts the *viewer* follows. That figure is
  // only on screen when the viewer is looking at their own profile, which is
  // the one case where the count beside the name has to move with the button.
  const viewerIsOwner =
    !!viewer && viewer.toLowerCase() === owner.toLowerCase();

  // Per-tab state. Each tab remembers its own page and rows, so switching back
  // and forth costs nothing and does not lose a reader's place.
  const state = {
    followers: { page: 0, more: false, rows: [], loaded: false },
    following: { page: 0, more: false, rows: [], loaded: false },
  };
  let active = "followers";
  let loading = false;

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
  function rowEl(m) {
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
    if (m.online) {
      const tag = document.createElement("span");
      tag.className = "follow-tag";
      tag.textContent = "online";
      link.appendChild(tag);
    }
    row.appendChild(link);

    // No button for a signed-out reader (there is no account to follow from)
    // and none on the viewer's own row (the server refuses a self-follow, and
    // offering the control would be a promise it will not keep).
    if (viewer && !m.self) row.appendChild(toggleEl(m));
    return row;
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

  function render() {
    const s = state[active];
    list.replaceChildren();
    if (!s.rows.length) {
      list.appendChild(
        empty(
          active === "followers"
            ? "Nobody is following " + owner + " yet."
            : owner + " is not following anybody yet."
        )
      );
    } else {
      const ul = document.createElement("ul");
      ul.className = "follow-rows";
      s.rows.forEach(function (m) {
        ul.appendChild(rowEl(m));
      });
      list.appendChild(ul);
    }
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
})();
