// Unread-feedback badge polling: keeps the red dot on the header name and on
// the profile popover's System link current on a page nobody is reloading.
//
// Loaded only for moderators (scriptsBase in view/scripts.templ) — they are the
// only viewer the dot can ever appear for — so there is no cost at all for
// everyone else, and no permission check here beyond the server's.
//
// The dot is created and removed rather than shown and hidden. That is exactly
// what the server-rendered markup does (unreadDot in view/feedback.templ emits
// nothing at zero), so a page that has been polling and a page freshly loaded
// agree on their DOM instead of drifting into two ways of saying "none".
(function () {
  "use strict";

  const anchors = document.querySelectorAll("[data-unread-anchor]");
  if (!anchors.length) return;

  // The server caches the count for its own short TTL, so asking faster than
  // that just re-reads the same number. A minute suits a channel where nothing
  // is urgent and keeps a moderator with several tabs open well inside the
  // shared /api/mod rate limit.
  const pollMs = 60000;
  // A tab flick should not re-ask when the answer cannot have moved yet.
  const minGapMs = 15000;

  let timer = null;
  let lastFetch = 0;
  let inFlight = false;

  function render(n, label) {
    anchors.forEach(function (el) {
      let dot = el.querySelector(".unread-dot");
      if (n > 0) {
        if (!dot) {
          dot = document.createElement("span");
          dot.className = "unread-dot";
          dot.setAttribute("role", "status");
          el.appendChild(dot);
        }
        dot.title = label;
        dot.setAttribute("aria-label", label);
      } else if (dot) {
        dot.remove();
      }
    });
  }

  function stop() {
    if (timer) {
      clearInterval(timer);
      timer = null;
    }
  }

  function start() {
    stop();
    timer = setInterval(function () { poll(true); }, pollMs);
  }

  async function poll(force) {
    if (inFlight) return;
    if (!force && Date.now() - lastFetch < minGapMs) return;
    inFlight = true;
    try {
      const res = await fetch("/api/mod/feedback/unread", {
        headers: { Accept: "application/json" },
      });
      // Signed out or demoted mid-session: this page will never be allowed to
      // read the count again, so stop asking rather than polling a 401/404
      // once a minute forever. The stale dot goes on the next navigation.
      if (res.status === 401 || res.status === 403 || res.status === 404) {
        stop();
        return;
      }
      // any other non-OK (a blip, a 429) leaves the badge alone and retries on
      // the next tick — a correct badge must not be blanked by a bad answer
      if (!res.ok) return;

      const data = await res.json();
      if (data && typeof data.unread === "number") {
        render(data.unread, data.label || "");
      }
      lastFetch = Date.now();
    } catch (e) {
      // network blip; same reasoning as above — leave what is on screen
    } finally {
      inFlight = false;
    }
  }

  // Only the visible tab polls: a moderator with a dozen tabs open should not
  // multiply the request rate, and a hidden tab's badge is not being read.
  // Returning to a tab is when the number matters most, so refresh then —
  // guarded by minGapMs so flicking between tabs cannot spam the endpoint.
  document.addEventListener("visibilitychange", function () {
    if (document.hidden) {
      stop();
      return;
    }
    poll(false);
    start();
  });

  // No poll on load: the page was just rendered with a fresh count, so the
  // first one is due a full interval from now.
  if (!document.hidden) start();
})();
