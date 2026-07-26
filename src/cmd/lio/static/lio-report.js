// Player reporting (arch/ADMIN_MODERATION.md Phase 4): the dialog behind the
// "Report <opponent>" control on the game-over panel.
//
// This is the one moderation surface an ordinary player sees, and most will see
// it once. So it says what will happen in plain terms, confirms that the report
// landed, and then gets out of the way — there is no status to track, no appeal
// thread, and deliberately no feedback about what a moderator decided.
//
// The server re-checks everything the control's visibility implies (logged in,
// opponent is a real account, not yourself); nothing here is a gate.
(function () {
  "use strict";

  const modal = document.getElementById("modalReport");
  const openBtn = document.getElementById("result-report");
  if (!modal || !openBtn) return;

  const form = document.getElementById("reportForm");
  const targetEl = document.getElementById("reportTarget");
  const categoryEl = document.getElementById("reportCategory");
  const noteEl = document.getElementById("reportNote");
  const errorEl = document.getElementById("reportError");
  const okEl = document.getElementById("reportOk");
  const submitBtn = document.getElementById("reportSubmit");
  const cancelBtn = document.getElementById("reportCancel");

  const target = openBtn.dataset.reportTarget || "";
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

  function open() {
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

  openBtn.addEventListener("click", open);
  cancelBtn.addEventListener("click", close);
  form.addEventListener("submit", send);
  modal.addEventListener("click", function (ev) {
    if (ev.target === modal || ev.target.closest(".modal-close")) close();
  });
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape" && modal.classList.contains("open")) close();
  });
})();
