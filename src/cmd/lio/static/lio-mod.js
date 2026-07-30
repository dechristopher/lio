// Moderation UI (arch/ADMIN_MODERATION.md): the bar on a player page and the
// site controls on /system.
//
// Both drive the same confirmation modal — one that names the specific change,
// who or what it lands on, and its consequence, then asks for the reason. The
// flow it replaced put a reason field at the top of each form (asking for a
// justification before the operator had chosen what to justify) and confirmed
// with a browser confirm() that named neither the action nor its effect.
//
// Deliberately thin beyond that: post, then reload. There is no optimistic UI
// and no local state — a moderation action must be shown as the server actually
// recorded it, not as the page hoped it went. Nothing here is a security control
// either: these surfaces only render for a viewer the server already judged
// privileged, and every endpoint re-checks that independently.
(function () {
  "use strict";

  const modal = document.getElementById("modalConfirmChange");
  if (!modal) return;

  const summary = document.getElementById("confirmSummary");
  const confirmForm = document.getElementById("confirmForm");
  const reasonInput = document.getElementById("confirmReason");
  const errorEl = document.getElementById("confirmError");
  const applyBtn = document.getElementById("confirmApply");

  // the two surfaces; either may be absent depending on the page
  const modForm = document.getElementById("modForm");
  const settingsForm = document.getElementById("settingsForm");

  // { kind: "mod" | "setting", btn } awaiting confirmation, or null when closed
  let pending = null;
  // Guards against a second submit while one is in flight. Disabling the button
  // is not enough on its own: form.requestSubmit() (and Enter in the reason
  // field) fires the submit event whether or not the button is disabled, and
  // every action here writes a permanent audit entry — a double-press would
  // leave two records of one decision.
  let submitting = false;

  // Make the server's rendered markup authoritative on every load.
  //
  // Both surfaces apply a change and then reload, and browsers restore form
  // field values across a reload — so after clearing the site notice with its X
  // the input would still show the text that was just deleted, and after a
  // title change the picker would still show the previous title. The server
  // renders the truth into the value/selected attributes; form.reset() puts
  // every control back to exactly those, discarding the browser's copy.
  //
  // Safe at init: nothing has been typed yet on a fresh load, and a bfcache
  // restore does not re-run this script (that path is handled separately, by
  // the pageshow reset in navScript).
  [modForm, settingsForm].forEach(function (f) {
    if (f) f.reset();
  });

  function field(form, name) {
    if (!form) return "";
    const el = form.elements[name];
    return el ? el.value.trim() : "";
  }

  function selectedText(form, name) {
    if (!form) return "";
    const el = form.elements[name];
    if (!el || el.selectedIndex < 0) return "";
    return el.options[el.selectedIndex].text.trim();
  }

  function setError(text) {
    if (!errorEl) return;
    errorEl.textContent = text || "";
    errorEl.classList.toggle("hidden", !text);
  }

  // --- what each action will do, in the operator's terms ---------------------

  // A player-page action. The server renders the consequence onto the button
  // (data-effect); the specific value comes from whichever control feeds it, so
  // the summary shows the title/role/name actually chosen rather than just the
  // verb. The account name is always included: acting on the wrong person is the
  // classic mistake, and a moderator may have several tabs open.
  function describeMod(btn) {
    const who = modForm ? modForm.dataset.username || "" : "";
    const action = btn.dataset.modAction;
    let label = btn.dataset.confirm || "";
    let value = "";

    switch (action) {
      case "title": {
        const chosen = field(modForm, "title");
        label = chosen ? "Set title" : "Remove title";
        value = chosen ? selectedText(modForm, "title") : "";
        break;
      }
      case "role":
        label = "Change role";
        value = field(modForm, "role");
        break;
      case "rename":
        label = "Rename account";
        value = field(modForm, "username");
        break;
      case "ban":
        label = "Ban account";
        value = selectedText(modForm, "duration");
        break;
      case "unban":
        label = label || "Lift the ban";
        break;
    }
    if (who) label += " — " + who;
    return { label: label, value: value, effect: btn.dataset.effect || "" };
  }

  // A /system site control. Everything is server-rendered except the notice,
  // whose summary depends on what was just typed.
  function describeSetting(btn) {
    let effect = btn.dataset.effect || "";
    let value = "";
    if (btn.dataset.setting === "notice") {
      // an explicit empty data-value is the Active-notices stand-down
      const text = btn.dataset.value === undefined ? field(settingsForm, "noticeText") : "";
      if (text) {
        value = text;
      } else {
        effect = "The banner disappears from every page.";
      }
    }
    return {
      label: btn.dataset.confirm || "Apply this change",
      value: value,
      effect: effect,
    };
  }

  // A queue or ops action. Both are single-button: the server-rendered
  // data-confirm already names the specific report or room, so there is no
  // field to read back.
  function describeSimple(btn) {
    return {
      label: btn.dataset.confirm || "Apply this action",
      value: "",
      effect: btn.dataset.effect || "",
    };
  }

  // --- modal -----------------------------------------------------------------

  function open(kind, btn) {
    let d;
    if (kind === "mod") {
      d = describeMod(btn);
    } else if (kind === "setting") {
      d = describeSetting(btn);
    } else {
      d = describeSimple(btn);
    }

    // Refuse to open on an action whose input is empty rather than confirming a
    // change the server would reject — a rename to nothing.
    if (kind === "mod" && btn.dataset.modAction === "rename" && !d.value) {
      const el = modForm.elements["username"];
      if (el) el.focus();
      return;
    }

    pending = { kind: kind, btn: btn };
    summary.innerHTML = "";

    const head = document.createElement("p");
    head.className = "confirm-change";
    head.textContent = d.label;
    summary.appendChild(head);

    if (d.value) {
      const quote = document.createElement("p");
      quote.className = "confirm-value";
      quote.textContent = d.value;
      summary.appendChild(quote);
    }
    if (d.effect) {
      const eff = document.createElement("p");
      eff.className = "confirm-effect";
      eff.textContent = d.effect;
      summary.appendChild(eff);
    }

    setError("");
    reasonInput.value = "";
    submitting = false;
    applyBtn.disabled = false;
    modal.classList.add("open");
    reasonInput.focus();
  }

  function close() {
    pending = null;
    modal.classList.remove("open");
  }

  // --- submission ------------------------------------------------------------

  function modBody(reason) {
    return {
      userId: Number(modForm.dataset.userId),
      reason: reason,
      duration: field(modForm, "duration"),
      titleId: field(modForm, "title"),
      role: field(modForm, "role"),
      username: field(modForm, "username"),
    };
  }

  // only the field being changed, so a form left open in a tab can never
  // silently restate the rest of the site's configuration
  function settingBody(btn, reason) {
    const body = { reason: reason };
    const setting = btn.dataset.setting;
    if (setting === "notice") {
      body.noticeText =
        btn.dataset.value === undefined ? field(settingsForm, "noticeText") : "";
      body.noticeLevel = field(settingsForm, "noticeLevel");
      return body;
    }
    body[setting] = btn.dataset.value === "1";
    return body;
  }

  async function apply() {
    if (!pending || submitting) return;
    const reason = reasonInput.value.trim();
    if (!reason) {
      setError("A reason is required.");
      reasonInput.focus();
      return;
    }

    const btn = pending.btn;
    const isMod = pending.kind === "mod";
    let url, body;
    switch (pending.kind) {
      case "mod":
        url = "/api/mod/" + btn.dataset.modAction;
        body = modBody(reason);
        break;
      case "setting":
        url = "/api/mod/settings";
        body = settingBody(btn, reason);
        break;
      case "report":
        url = "/api/mod/report/resolve";
        body = { id: Number(btn.dataset.resolveReport), resolution: reason };
        break;
      case "room":
        url = "/api/mod/room/close";
        body = { roomId: btn.dataset.closeRoom, reason: reason };
        break;
    }

    submitting = true;
    applyBtn.disabled = true;
    setError("");
    try {
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (res.ok) {
        // a rename changes this page's own URL, so follow it rather than
        // reloading a path that no longer resolves
        if (isMod && btn.dataset.modAction === "rename") {
          const data = await res.json().catch(() => null);
          if (data && data.username) {
            window.location.href = "/@/" + encodeURIComponent(data.username);
            return;
          }
        }
        // reload so the rendered state (including the site banners, which are on
        // every page) is exactly what the server now holds
        window.location.reload();
        return;
      }
      const err = await res.json().catch(() => null);
      setError((err && err.error) || "That action failed.");
      submitting = false;
      applyBtn.disabled = false;
    } catch (e) {
      setError("Network error — nothing was applied.");
      submitting = false;
      applyBtn.disabled = false;
    }
  }

  // --- wiring ----------------------------------------------------------------

  // Delegated at the document: the /system stand-down buttons sit in their own
  // card outside the settings form but drive the same changes.
  document.addEventListener("click", function (ev) {
    const modBtn = ev.target.closest("[data-mod-action]");
    if (modBtn && modForm) {
      ev.preventDefault();
      open("mod", modBtn);
      return;
    }
    const setBtn = ev.target.closest("[data-setting]");
    if (setBtn && settingsForm) {
      ev.preventDefault();
      open("setting", setBtn);
      return;
    }
    const repBtn = ev.target.closest("[data-resolve-report]");
    if (repBtn) {
      ev.preventDefault();
      open("report", repBtn);
      return;
    }
    const roomBtn = ev.target.closest("[data-close-room]");
    if (roomBtn) {
      ev.preventDefault();
      open("room", roomBtn);
      return;
    }
    if (!modal.classList.contains("open")) return;
    if (ev.target === modal || ev.target.closest("#confirmCancel") ||
        ev.target.closest("#modalConfirmChange .modal-close")) {
      ev.preventDefault();
      close();
    }
  });

  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape" && modal.classList.contains("open")) close();
  });

  confirmForm.addEventListener("submit", function (ev) {
    ev.preventDefault();
    apply();
  });

  // neither bar is a real form submission; Enter must never navigate
  [modForm, settingsForm].forEach(function (f) {
    if (f) f.addEventListener("submit", function (ev) { ev.preventDefault(); });
  });
})();

// Pause the instance panel's self-poll while the tab is hidden. Each tick
// probes Postgres, Redis and the object store, so a console left open in a
// background tab would keep three services busy for nobody's benefit.
//
// This is an htmx:beforeRequest listener rather than an hx-trigger filter
// because htmx compiles filter expressions with the Function constructor, which
// the site's CSP (no 'unsafe-eval') refuses — the inline form throws on every
// page load and gates nothing.
(function () {
  document.addEventListener("htmx:beforeRequest", function (ev) {
    var el = ev.detail && ev.detail.elt;
    if (!el || el.id !== "systemStats") return;
    if (document.visibilityState !== "visible") ev.preventDefault();
  });
})();

// The operator message composer on /system (arch/NOTIFICATIONS.md Phase 3):
// find one player, write to them, send.
//
// Its own IIFE because it shares nothing with the confirmation flow above —
// which returns early on any page without the confirm modal, and would take
// this with it. Nothing here is a security control: both endpoints re-check the
// caller's role.
//
// No confirmation dialog, unlike the moderation actions. Those change what an
// account may do and are hard to undo; this sends a message, and asking someone
// to confirm every message would train them to click through the dialog that
// guards the bans.
(function () {
  "use strict";

  const search = document.getElementById("msgSearch");
  const results = document.getElementById("msgResults");
  const picked = document.getElementById("msgPicked");
  const bodyEl = document.getElementById("msgBody");
  const sendBtn = document.getElementById("msgSend");
  const errorEl = document.getElementById("msgError");
  const okEl = document.getElementById("msgOk");
  if (!search || !results || !sendBtn) return;

  // The shortest term the server answers, mirrored here so a single keystroke
  // costs no request at all.
  const minTerm = 2;
  // Long enough that a typed word settles before it is looked up, short enough
  // that the list still feels like it is following along.
  const debounceMs = 200;

  let target = null; // {id, username}, or null while nobody is chosen
  let timer = null;
  let seq = 0; // guards against an older search landing after a newer one

  function setStatus(el, text) {
    if (!el) return;
    el.textContent = text || "";
    el.classList.toggle("hidden", !text);
  }

  function clearStatus() {
    setStatus(errorEl, "");
    setStatus(okEl, "");
  }

  function syncSend() {
    const body = bodyEl ? bodyEl.value.trim() : "";
    sendBtn.disabled = !target || body.length < 8;
  }

  function choose(player) {
    target = player;
    results.replaceChildren();
    search.value = "";
    if (picked) {
      picked.textContent = "Writing to " + player.username;
      picked.classList.remove("hidden");
    }
    clearStatus();
    syncSend();
    if (bodyEl) bodyEl.focus();
  }

  function renderResults(players) {
    results.replaceChildren();
    if (!players.length) return;
    players.forEach(function (p) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "msg-result";
      btn.setAttribute("role", "option");
      btn.textContent = p.username;
      btn.addEventListener("click", function () { choose(p); });
      results.appendChild(btn);
    });
  }

  async function runSearch(term) {
    const mine = ++seq;
    try {
      const res = await fetch("/api/mod/users/search?q=" + encodeURIComponent(term), {
        headers: { Accept: "application/json" },
      });
      if (!res.ok || mine !== seq) return;
      const data = await res.json();
      // a late answer to a term the operator has already typed past would
      // replace the list under them
      if (mine === seq) renderResults((data && data.players) || []);
    } catch (e) {
      // a failed lookup leaves the last list alone; typing again retries
    }
  }

  search.addEventListener("input", function () {
    // Typing again means the previously chosen player is no longer the subject.
    // Leaving them selected would let somebody search for a second name, not
    // click it, and send to the first.
    target = null;
    if (picked) picked.classList.add("hidden");
    syncSend();
    clearStatus();

    const term = search.value.trim();
    clearTimeout(timer);
    if (term.length < minTerm) {
      results.replaceChildren();
      return;
    }
    timer = setTimeout(function () { runSearch(term); }, debounceMs);
  });

  if (bodyEl) bodyEl.addEventListener("input", function () { clearStatus(); syncSend(); });

  sendBtn.addEventListener("click", async function () {
    if (!target) return;
    const body = bodyEl ? bodyEl.value.trim() : "";
    clearStatus();
    // Held down for the whole request: this writes an audit entry and a
    // notification, and a double press would send the message twice.
    sendBtn.disabled = true;
    try {
      const res = await fetch("/api/mod/notify", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ userId: target.id, body: body }),
      });
      const data = await res.json().catch(function () { return null; });
      if (!res.ok) {
        setStatus(errorEl, (data && data.error) || "could not send that message");
        syncSend();
        return;
      }
      setStatus(okEl, "Sent to " + ((data && data.sent) || target.username) + ".");
      // Reset to an empty composer: the next message is to somebody else more
      // often than it is a second message to the same person.
      target = null;
      if (picked) picked.classList.add("hidden");
      if (bodyEl) bodyEl.value = "";
      search.value = "";
      results.replaceChildren();
    } catch (e) {
      setStatus(errorEl, "could not send that message");
    } finally {
      syncSend();
    }
  });

  syncSend();
})();
