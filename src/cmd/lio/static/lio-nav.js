// Header and footer chrome: the account/preferences/notification popovers, the
// mobile footer nav, and the create-game dialog.
//
// These three were inline <script> blocks in the header, footer and modal
// components until they were pulled out here. Inline, they shipped ~15KB of
// identical JS in the HTML of every single page and could never be cached. As
// one hashed file the browser fetches them once and reuses them site-wide.
//
// Loaded with `defer` from the document head (layout.templ), so the fetch
// starts during head parsing without blocking it, and runs after the DOM is
// parsed but before DOMContentLoaded.
//
// Two invariants this file depends on, both of which it keeps:
//
//  1. Nothing here touches the DOM at init — every block only defines
//     functions and attaches listeners. Initial visual state is entirely
//     server-rendered (popovers ship with `hidden`, modals without `.open`),
//     so running after first paint can move nothing on the page.
//  2. Nothing here defines a global another script needs while it evaluates.
//     window.__resetProfilePopover is the one export, and its callers
//     (lio-auth.js, lio-feedback.js) both guard it and only reach it from
//     user actions. The globals that *are* consumed at socket-connect time —
//     window.lioConn and window.lioUpdateNotice — deliberately stay inline in
//     the header for that reason.
//
// Whichever block runs, each guards for its own markup: the header is shared
// but not every element inside it renders for every viewer (the account modal
// is logged-out-only, the bell and following popover logged-in-only).

// ---- header popovers, account modal, preferences ----

(function () {
    const modal = document.getElementById("modalAccount");
    const loginBtn = document.getElementById("loginButton");
    // the username text (logged in only) opens the profile popover
    const profileBtns = ["profileButton"]
        .map((id) => document.getElementById(id)).filter(Boolean);
    const profilePop = document.getElementById("profilePopover");
    const prefsBtn = document.getElementById("prefsButton");
    const popover = document.getElementById("prefsPopover"); // preferences
    const notifyBtn = document.getElementById("notifyButton");
    const notifyPop = document.getElementById("notifyPanel"); // notifications
    const followBtn = document.getElementById("followingButton");
    const followPop = document.getElementById("followingPanel"); // people you follow
    const scrim = document.getElementById("menuScrim");

    const isOpen = (el) => el && !el.classList.contains("hidden");
    const closePrefs = () => { if (popover) popover.classList.add("hidden"); };
    const closeNotify = () => {
        if (!notifyPop) return;
        notifyPop.classList.add("hidden");
        if (notifyBtn) notifyBtn.setAttribute("aria-expanded", "false");
    };
    const closeFollow = () => {
        if (!followPop) return;
        followPop.classList.add("hidden");
        if (followBtn) followBtn.setAttribute("aria-expanded", "false");
    };
    // Restore the account popover to its pristine state whenever it is
    // dismissed: clear every form field, collapse the expandable sections,
    // and hide transient error/success lines. Dispatching an input event
    // lets each form's own controller (lio-auth.js) recompute derived
    // state — e.g. the change-password submit/confirm gating.
    const resetProfile = () => {
        if (!profilePop) return;
        profilePop.querySelectorAll("form").forEach((f) => f.reset());
        profilePop.querySelectorAll("details").forEach((d) => { d.open = false; });
        profilePop.querySelectorAll("[data-auth-error],[data-auth-ok]")
            .forEach((el) => el.classList.add("hidden"));
        profilePop.querySelectorAll("input")
            .forEach((i) => i.dispatchEvent(new Event("input", { bubbles: true })));
    };
    // expose so lio-auth.js can reset the popover when it hides it to open
    // the edit-profile / security modals
    window.__resetProfilePopover = resetProfile;
    const closeProfile = () => {
        if (!profilePop) return;
        if (isOpen(profilePop)) resetProfile();
        profilePop.classList.add("hidden");
    };
    // the backdrop follows the menus: shown (dim + blur) whenever either
    // header popover is open, hidden once both are closed
    const syncScrim = () => {
        if (scrim) scrim.classList.toggle("is-open",
            isOpen(popover) || isOpen(profilePop) || isOpen(notifyPop) || isOpen(followPop));
    };
    const closeMenus = () => { closePrefs(); closeProfile(); closeNotify(); closeFollow(); syncScrim(); };

    const closeModal = () => { if (modal) modal.classList.remove("open"); };
    if (modal) {
        const closeBtn = modal.querySelector(".modal-close");
        if (loginBtn) loginBtn.addEventListener("click", () => modal.classList.add("open"));
        if (closeBtn) closeBtn.addEventListener("click", closeModal);
        modal.addEventListener("click", (e) => { if (e.target === modal) closeModal(); });
    }

    // Ring the swatch/pill matching the live <html> data-board/data-piece,
    // and highlight the selected theme mode. The mode is derived from
    // storage, not <html> data-theme: an explicit choice persists a
    // "theme" key, while System is its absence (data-theme always holds
    // the *resolved* light/dark, which can't distinguish the two).
    const syncPrefs = () => {
        if (!popover) { return; }
        const root = document.documentElement.dataset;
        popover.querySelectorAll("[data-set-board]").forEach((b) =>
            b.classList.toggle("is-active", b.dataset.setBoard === root.board));
        popover.querySelectorAll("[data-set-piece]").forEach((b) =>
            b.classList.toggle("is-active", b.dataset.setPiece === root.piece));
        let mode = "system";
        try {
            const t = localStorage.getItem("theme");
            if (t === "light" || t === "dark") mode = t;
        } catch (e) {}
        popover.querySelectorAll("[data-set-theme]").forEach((b) =>
            b.classList.toggle("is-active", b.dataset.setTheme === mode));
    };
    // Delegated: a swatch sets the board theme, a pill sets the piece set,
    // a theme button sets (or clears, for System) the color scheme.
    // __setBoard/__setPiece/__setTheme/__useSystemTheme (layout.templ)
    // flip the <html> attribute + persist; the CSS reacts instantly.
    if (popover) popover.addEventListener("click", (e) => {
        const boardBtn = e.target.closest("[data-set-board]");
        if (boardBtn) { window.__setBoard(boardBtn.dataset.setBoard); syncPrefs(); return; }
        const pieceBtn = e.target.closest("[data-set-piece]");
        if (pieceBtn) { window.__setPiece(pieceBtn.dataset.setPiece); syncPrefs(); return; }
        const themeBtn = e.target.closest("[data-set-theme]");
        if (themeBtn) {
            const v = themeBtn.dataset.setTheme;
            if (v === "system") { window.__useSystemTheme(); } else { window.__setTheme(v); }
            syncPrefs();
        }
    });

    // ---- account-backed preferences -----------------------------------
    // The theme/board/piece controls above persist to localStorage, because
    // they resolve before first paint. These persist to the account instead,
    // so they follow the player to their next device, and are therefore a
    // round trip rather than a synchronous write.
    //
    // Two controls drive the same preference: the switch in this popover, and
    // the × on the thing itself (the home page's "What is Octad?" card). Both
    // land here, so there is one place that knows what a preference does to
    // the page it is currently on.
    const savePref = (key, on) =>
        fetch("/api/me/prefs", {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify({key: key, on: on}),
        }).then((r) => {
            if (!r.ok) throw new Error("pref");
        });

    // What each preference does to the page in front of the viewer right now.
    // Turning one *off* is applied in place — the card is simply taken out.
    // Turning one back *on* cannot be: the markup is server-rendered and this
    // page was built without it, so the home page reloads to fetch it and any
    // other page has nothing to do until its next load.
    const prefEffects = {
        "home.about": (on) => {
            const card = document.getElementById("homeAbout");
            if (on) {
                if (!card && document.getElementById("home-activity")) {
                    window.location.reload();
                }
                return;
            }
            if (!card) return;
            // the demo board keeps a timer and refetches; a detached one would
            // keep doing both for as long as the page stayed open
            if (window.lioHomeDemoStop) window.lioHomeDemoStop();
            card.remove();
        },
    };
    const applyPref = (key, on) => {
        const effect = prefEffects[key];
        if (effect) effect(on);
    };

    // The popover switch. The box carries the state, so a failed write puts it
    // back rather than leaving the control claiming something the account does
    // not say.
    if (popover) popover.addEventListener("change", (e) => {
        const box = e.target.closest("[data-pref]");
        if (!box) return;
        const row = box.closest(".pref-toggle");
        const key = box.dataset.pref;
        const on = box.checked;
        if (row) row.classList.add("is-saving");
        savePref(key, on)
            .then(() => applyPref(key, on))
            .catch(() => { box.checked = !on; })
            .then(() => { if (row) row.classList.remove("is-saving"); });
    });

    // The × on a dismissible card. Delegated from the document because the
    // card is elsewhere on the page, and only appears on some pages. The card
    // stays put until the write lands: it comes back on the next load anyway
    // if the write failed, and hiding it first would promise otherwise.
    document.addEventListener("click", (e) => {
        const btn = e.target.closest("[data-pref-off]");
        if (!btn) return;
        const key = btn.dataset.prefOff;
        btn.disabled = true;
        savePref(key, false)
            .then(() => {
                // keep the popover's switch honest without reopening it
                const box = popover && popover.querySelector("[data-pref='" + key + "']");
                if (box) box.checked = false;
                applyPref(key, false);
            })
            .catch(() => { btn.disabled = false; });
    });

    // Opening either popover closes the other first, so the two never
    // stack; the scrim then reflects whether anything is open.
    if (prefsBtn && popover) prefsBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        closeProfile();
        closeNotify();
        closeFollow();
        popover.classList.toggle("hidden");
        if (isOpen(popover)) syncPrefs();
        syncScrim();
    });
    // The bell. Opening it tells the notification client, which fetches
    // the list on the first open and clears the badge — the dot means
    // "something is new", and it has now been seen. The rows keep their
    // own unread mark until they are read, a different question.
    if (notifyPop && notifyBtn) notifyBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        closePrefs();
        closeProfile();
        closeFollow();
        if (isOpen(notifyPop)) {
            closeNotify();
        } else {
            notifyPop.classList.remove("hidden");
            notifyBtn.setAttribute("aria-expanded", "true");
            if (window.__notifyPanelOpened) window.__notifyPanelOpened();
        }
        syncScrim();
    });
    // The following control. Like the bell, opening it tells its client,
    // which fetches the list on the first open and corrects the badge
    // the header painted at render time — presence moves, and this is
    // the moment the number can be made true again.
    if (followPop && followBtn) followBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        closePrefs();
        closeProfile();
        closeNotify();
        if (isOpen(followPop)) {
            closeFollow();
        } else {
            followPop.classList.remove("hidden");
            followBtn.setAttribute("aria-expanded", "true");
            if (window.__followingPanelOpened) window.__followingPanelOpened();
        }
        syncScrim();
    });
    if (profilePop) profileBtns.forEach((btn) => btn.addEventListener("click", (e) => {
        e.stopPropagation();
        closePrefs();
        closeNotify();
        closeFollow();
        // close (with reset) when open, otherwise reveal — a plain class
        // toggle would bypass closeProfile's reset on the closing click
        if (isOpen(profilePop)) closeProfile();
        else profilePop.classList.remove("hidden");
        syncScrim();
    }));

    // the backdrop dismisses any open menu on click
    if (scrim) scrim.addEventListener("click", closeMenus);
    // safety net for outside clicks not caught by the scrim
    document.addEventListener("click", (e) => {
        const within =
            (popover && popover.contains(e.target)) ||
            (profilePop && profilePop.contains(e.target)) ||
            (notifyPop && notifyPop.contains(e.target)) ||
            (followPop && followPop.contains(e.target)) ||
            (prefsBtn && prefsBtn.contains(e.target)) ||
            (notifyBtn && notifyBtn.contains(e.target)) ||
            (followBtn && followBtn.contains(e.target)) ||
            profileBtns.some((btn) => btn.contains(e.target));
        if (!within) closeMenus();
    });
    document.addEventListener("keydown", (e) => {
        if (e.key === "Escape") { closeModal(); closeMenus(); }
    });

    // Back/forward restores land here with the DOM exactly as it was left,
    // so a menu that was open when the visitor navigated away is still open
    // when they come back — and since the script does not re-run, nothing
    // closes it. Now that the profile popover contains real navigation
    // (System / Moderation), that is easy to hit: open the menu, follow a
    // link, press Back, and the menu is sitting there over the page with
    // the scrim dimming everything behind it.
    //
    // On a restore, any open overlay is by definition stale: no script has
    // run to open it. Nothing opens a menu or modal at load time, so this
    // can never undo something a page meant to show. Guarded on persisted
    // because a normal load re-runs everything already closed.
    window.addEventListener("pageshow", (e) => {
        if (!e.persisted) return;
        closeMenus();
        closeModal();
        // the other modals belong to other scripts (auth, create-game, bot
        // difficulty, moderation), but the rule is the same for all of them
        document.querySelectorAll(".modal-shade.open")
            .forEach((m) => m.classList.remove("open"));
    });
})();

// ---- footer navigation ----

(function () {
    if (window.__footerNavInit) { return; }
    window.__footerNavInit = true;
    function reflow() {
        document.querySelectorAll("[data-footer-nav]").forEach(function (nav) {
            var seps = nav.querySelectorAll(":scope > span");
            seps.forEach(function (s) { s.style.visibility = ""; });
            seps.forEach(function (s) {
                var a = s.previousElementSibling, b = s.nextElementSibling;
                if (a && b && Math.abs(a.offsetTop - b.offsetTop) > 1) {
                    s.style.visibility = "hidden";
                }
            });
        });
    }
    window.addEventListener("resize", reflow);
    window.addEventListener("load", reflow);
    reflow();
})();

// ---- create-game dialog ----

(function () {
    const modal = document.getElementById("modalCreateGame");
    // The dialog is mounted by the shared header, so it is normally present.
    // This file is no longer co-located with that markup, though, so its
    // existence is an assumption rather than a guarantee: bail rather than
    // throw on a page that ever omits the header.
    if (!modal) { return; }
    const closeBtn = modal.querySelector(".modal-close");
    const publicBox = modal.querySelector(".cg-public-box");
    const humanOpp = modal.querySelector("#opp-human");
    const inviteField = modal.querySelector("#cg-invite");
    const kicker = modal.querySelector("[data-cg-kicker]");
    const titleEl = modal.querySelector("[data-cg-title]");
    // Challenge mode is a property of one opening, not of the page: the
    // modal is a single element reused by every trigger, so it is reset
    // on the way in rather than on the way out. Resetting on close would
    // leave a stale target on any path that dismisses it some other way
    // (Escape, the scrim, a bfcache restore).
    const form = modal.querySelector("form.cg");
    const botOpp = modal.querySelector("#opp-computer");
    const anonBox = modal.querySelector(".cg-allow-anon-box");
    const setChallenge = (name) => {
        if (inviteField) inviteField.value = name || "";
        if (kicker) kicker.textContent = name ? "Challenge" : "New game";
        if (titleEl) titleEl.textContent = name ? "Challenging " + name : "Create a game";
        // is-challenge hides the three settings a challenge already answers
        // (opponent, open challenge, allow anonymous — see app.css).
        if (form) form.classList.toggle("is-challenge", !!name);
        // Hidden is not the same as harmless: these controls still submit
        // whatever they were left holding by an earlier opening of this same
        // dialog. Put them into the state a challenge means, every time.
        // Disabling the Computer radio guards the one that would otherwise
        // be reachable by keyboard if the styles ever failed to load.
        if (botOpp) botOpp.disabled = !!name;
        if (name) {
            if (humanOpp) humanOpp.checked = true;
            if (publicBox) publicBox.checked = false;
            if (anonBox) anonBox.checked = false;
        }
    };
    const open = () => modal.classList.add("open");
    const close = () => modal.classList.remove("open");
    // Delegated open: any [data-open-create-game] element opens the modal,
    // including buttons that htmx swaps into the live home-activity region
    // after the initial page load. A trigger carrying [data-prefill-public]
    // (the Open Challenges "+ New" / "create one" affordances) pre-selects an
    // Open Challenge: force a human opponent (public requires it) and check
    // the toggle so the modal opens already set up to publish a seek.
    document.addEventListener("click", (e) => {
        const trigger = e.target.closest("[data-open-create-game]");
        if (!trigger) return;
        // A challenge is addressed to one person, so it is human-only, never
        // a public seek, and never open to anonymous joiners. setChallenge
        // puts all three into that state; the server forces them too.
        const target = trigger.getAttribute("data-challenge");
        setChallenge(target);
        if (!target && trigger.hasAttribute("data-prefill-public") && publicBox) {
            if (humanOpp) humanOpp.checked = true;
            publicBox.checked = true;
        }
        open();
    });
    if (closeBtn) closeBtn.addEventListener("click", close);
    modal.addEventListener("click", (e) => { if (e.target === modal) close(); });
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") close(); });

    // Resolve the chosen time control into the hidden field the form
    // submits. Every game is the blind-deploy variant now (no mode toggle),
    // so each card carries a single data-variant. Runs on time-control change
    // so the value is current before the color submit buttons (gated until a
    // time control is picked) are clickable.
    const variantField = modal.querySelector("#cg-variant");
    const syncVariant = () => {
        // :enabled — a card disabled by casual mode is not a choice, so
        // the submitted field goes empty (the server ignores it anyway)
        const tc = modal.querySelector(".tc-input:checked:enabled");
        variantField.value = tc ? tc.dataset.variant : "";
    };

    modal.querySelectorAll(".tc-input").forEach((el) => el.addEventListener("change", syncVariant));

    // Casual (untimed, any opponent) makes the time-control choice moot:
    // disable the cards while it is on so their `required` can't block
    // submission with none picked (CSS fades the section and opens the
    // color-submit gate). The server resolves the untimed casual variant
    // from the mode toggle instead. Race To is unaffected — a casual
    // human match can still race.
    const casualBox = modal.querySelector(".cg-casual-box");
    const syncCasual = () => {
        const on = !!casualBox && casualBox.checked;
        modal.querySelectorAll(".tc-input").forEach((el) => { el.disabled = on; });
        syncVariant();
    };
    if (casualBox) casualBox.addEventListener("change", syncCasual);

    // Bot games are never public open challenges nor race-to matches: clear
    // both when the computer is chosen (each control is faded + disabled in
    // place via CSS — nothing is removed, so switching opponents never
    // shifts the layout — and the server forces bot games private /
    // single-game regardless).
    modal.querySelectorAll("input[name=opponent]").forEach((el) =>
        el.addEventListener("change", () => {
            if (!modal.querySelector("#opp-computer").checked) return;
            if (publicBox) publicBox.checked = false;
            const raceOff = modal.querySelector("#race-0");
            if (raceOff) raceOff.checked = true;
        }));
})();
