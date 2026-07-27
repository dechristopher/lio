// Player reporting (arch/ADMIN_MODERATION.md Phase 4): the dialog behind every
// "Report <player>" control — the live game-over panel, the archive page's info
// bar, and a player's profile.
//
// This is the one moderation surface an ordinary player sees, and most will see
// it once. So it says what will happen in plain terms, confirms that the report
// landed, and then gets out of the way — there is no status to track, no appeal
// thread, and deliberately no feedback about what a moderator decided.
//
// The control is found by its data-report-target rather than by id, so a page
// that grows another entry point needs no change here; the attribute also
// carries *who* it reports, which is why the target is read per-click instead of
// once at load.
//
// The server re-checks everything the control's visibility implies (logged in,
// target is a real account, not yourself); nothing here is a gate.
(function () {
  "use strict";

  const modal = document.getElementById("modalReport");
  const openBtns = document.querySelectorAll("[data-report-target]");
  if (!modal || !openBtns.length) return;

  const form = document.getElementById("reportForm");
  const targetEl = document.getElementById("reportTarget");
  const categoryEl = document.getElementById("reportCategory");
  const noteEl = document.getElementById("reportNote");
  const errorEl = document.getElementById("reportError");
  const okEl = document.getElementById("reportOk");
  const submitBtn = document.getElementById("reportSubmit");
  const cancelBtn = document.getElementById("reportCancel");

  // whoever the control that opened the dialog names
  let target = "";
  // the game this report comes out of, when the page knows it — the archive
  // data block carries the id on a finished game
  const gameId = (function () {
    const el = document.getElementById("archive-data");
    if (el) {
      try { return (JSON.parse(el.textContent) || {}).gameId || ""; } catch (e) {}
    }
    return window.lioGameId || "";
  })();

  let submitting = false;

  function setError(text) {
    errorEl.textContent = text || "";
    errorEl.classList.toggle("hidden", !text);
  }

  function setOk(text) {
    okEl.textContent = text || "";
    okEl.classList.toggle("hidden", !text);
  }

  function open(btn) {
    target = btn.dataset.reportTarget || "";
    targetEl.textContent = target;
    setError("");
    setOk("");
    noteEl.value = "";
    categoryEl.selectedIndex = 0;
    submitting = false;
    submitBtn.disabled = false;
    form.classList.remove("hidden");
    modal.classList.add("open");
    categoryEl.focus();
  }

  function close() {
    modal.classList.remove("open");
  }

  async function send(ev) {
    ev.preventDefault();
    if (submitting) return;
    submitting = true;
    submitBtn.disabled = true;
    setError("");

    try {
      const res = await fetch("/api/report", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          username: target,
          category: categoryEl.value,
          note: noteEl.value,
          gameId: gameId,
        }),
      });

      if (res.status === 204 || res.ok) {
        // 200 with {already:true} is the duplicate case, which is not a
        // failure: the player has already told us, and saying so plainly beats
        // an error that invites them to try again.
        let message = "Thanks — a moderator will review this.";
        if (res.status === 200) {
          const data = await res.json().catch(() => null);
          if (data && data.message) message = data.message;
        }
        form.classList.add("hidden");
        setOk(message);
        // leave the confirmation up briefly, then get out of the way
        setTimeout(close, 2600);
        return;
      }
      const err = await res.json().catch(() => null);
      setError((err && err.error) || "Could not send that report.");
    } catch (e) {
      setError("Network error — the report was not sent.");
    }
    submitting = false;
    submitBtn.disabled = false;
  }

  openBtns.forEach(function (btn) {
    btn.addEventListener("click", function () { open(btn); });
  });
  cancelBtn.addEventListener("click", close);
  form.addEventListener("submit", send);
  modal.addEventListener("click", function (ev) {
    if (ev.target === modal || ev.target.closest(".modal-close")) close();
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape" && modal.classList.contains("open")) close();
  });

  // The archive page's info bar is a wrapping, centred flex row ("Archived
  // game · <date> · Report <player>"). Once the control no longer fits beside
  // the date, the separator ahead of it should go rather than dangle at the end
  // of the line above — and CSS cannot see a line break, so measure it.
  //
  // Always measure with the group in its natural state: the applied state
  // changes the group's width, so letting it feed into the next decision would
  // make the control flip between lines around the wrap point.
  (function () {
    const group = document.querySelector(".info-report-group");
    if (!group || !group.previousElementSibling) return;
    const prev = group.previousElementSibling;

    function syncWrap() {
      group.classList.remove("is-wrapped");
      if (group.offsetTop > prev.offsetTop) group.classList.add("is-wrapped");
    }

    syncWrap();
    window.addEventListener("resize", syncWrap);
    // a late webfont swap reflows the bar without ever firing resize
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(syncWrap);
  })();
})();
