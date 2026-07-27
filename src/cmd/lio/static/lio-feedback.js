// Site feedback: the dialog behind the "Tell us how it's going" prompt in the
// profile popover, plus the two mark-read controls on the /system inbox.
//
// Both halves of one small feature, in one file, because they are never both
// present: the dialog exists on every page for a logged-in viewer, and the
// inbox controls only on /system for a moderator. Each half self-guards on its
// own elements, so whichever is absent costs nothing.
//
// The dialog is deliberately low-ceremony. Someone volunteering that a button
// looks wrong on their phone should not have to fill in a form that feels like
// filing a police report — pick which of three things it is, type, send, done.
// Every rule the client applies is re-applied by the server; nothing here is a
// gate.
(function () {
  "use strict";

  // --- the send dialog -------------------------------------------------------

  const modal = document.getElementById("modalFeedback");
  const openBtn = document.getElementById("feedbackButton");

  if (modal && openBtn) {
    const form = document.getElementById("feedbackForm");
    const bodyEl = document.getElementById("feedbackBody");
    const errorEl = document.getElementById("feedbackError");
    const okEl = document.getElementById("feedbackOk");
    const submitBtn = document.getElementById("feedbackSubmit");
    const cancelBtn = document.getElementById("feedbackCancel");

    // matches the server's minBody; the point of checking here too is that the
    // person gets told before a round trip, not that the server trusts it
    const MIN_BODY = 8;

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
      setError("");
      setOk("");
      form.reset();
      submitting = false;
      submitBtn.disabled = false;
      form.classList.remove("hidden");
      modal.classList.add("open");
      // Close the popover this was opened from. It sits above the scrim, so
      // leaving it up would park the account menu on top of the dialog.
      if (window.__resetProfilePopover) window.__resetProfilePopover();
      const pop = document.getElementById("profilePopover");
      if (pop) pop.classList.add("hidden");
      const scrim = document.getElementById("menuScrim");
      if (scrim) scrim.classList.remove("is-open");
      bodyEl.focus();
    }

    function close() {
      modal.classList.remove("open");
    }

    function chosenKind() {
      const el = form.querySelector('input[name="kind"]:checked');
      return el ? el.value : "";
    }

    async function send(ev) {
      ev.preventDefault();
      if (submitting) return;

      const body = (bodyEl.value || "").trim();
      if (body.length < MIN_BODY) {
        setError("Tell us a little more than that.");
        bodyEl.focus();
        return;
      }

      submitting = true;
      submitBtn.disabled = true;
      setError("");

      try {
        const hp = form.elements["website"];
        const res = await fetch("/api/feedback", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            kind: chosenKind(),
            body: body,
            // where they were when they wrote it — the context that makes a
            // vague problem reproducible. Path only: a query string or hash
            // would be dropped server-side anyway.
            path: window.location.pathname,
            website: hp ? hp.value : "",
          }),
        });

        if (res.ok) {
          form.classList.add("hidden");
          setOk("Thanks — this goes straight to us.");
          setTimeout(close, 2600);
          return;
        }
        const err = await res.json().catch(() => null);
        setError((err && err.error) || "Could not send that.");
      } catch (e) {
        setError("Network error — nothing was sent.");
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
  }

  // --- the /system inbox controls -------------------------------------------

  // Thin like the rest of the moderation UI: post, then reload. Marking read
  // changes the header dot on every page, so the rendered state has to come
  // back from the server rather than being patched in place here.
  //
  // No confirmation step, unlike every other control on /system: those act on
  // people and demand a reason, while this one is a moderator saying "seen".
  // Putting a reason prompt in front of it would train them to stop clicking
  // it, which leaves the unread badge permanently lit and therefore useless.
  document.addEventListener("click", async function (ev) {
    const one = ev.target.closest("[data-feedback-read]");
    const all = ev.target.closest("[data-feedback-read-all]");
    if (!one && !all) return;

    ev.preventDefault();
    const btn = one || all;
    if (btn.disabled) return;
    btn.disabled = true;

    const url = one ? "/api/mod/feedback/read" : "/api/mod/feedback/read-all";
    const body = one ? { id: Number(one.dataset.feedbackRead) } : {};

    try {
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (res.ok) {
        window.location.reload();
        return;
      }
    } catch (e) {
      /* fall through to re-enable */
    }
    btn.disabled = false;
  });
})();
