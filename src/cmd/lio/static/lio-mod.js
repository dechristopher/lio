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
