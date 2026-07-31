// The bot difficulty picker: the persona ladder shown when a Quick Game button
// on the home page chooses the computer as opponent.
//
// Split out of an inline <script> in the botDifficultyModal component so it is
// cached rather than re-sent in the HTML. Only the home page mounts the dialog,
// so only the home page loads this file (index.templ).
//
// Like lio-nav.js: no DOM mutation at init, no globals other scripts wait on.
// It only defines functions and attaches listeners, so running after first
// paint cannot shift anything on the page.

(function () {
    const shade = document.getElementById("modalBotDifficulty");
    // Only the home page mounts this dialog, and only that page loads this
    // file. The guard costs nothing and keeps the pairing from being a
    // silent assumption now that the two are no longer co-located.
    if (!shade) { return; }
    const closeBtn = shade.querySelector(".modal-close");
    const STORE = "lio-bot-persona";
    let pendingForm = null;
    let pendingColor = null;

    const open = () => {
        // tag the remembered last-played persona so returning players
        // see their usual opponent at a glance
        let last = null;
        try { last = localStorage.getItem(STORE); } catch (e) {}
        shade.querySelectorAll(".bd-card").forEach((el) => {
            el.classList.toggle("bd-last", !!last && el.dataset.bot === last);
        });
        shade.classList.add("open");
    };
    const close = () => {
        shade.classList.remove("open");
        pendingForm = null;
        pendingColor = null;
    };

    // Hold any [data-bot-difficulty] form's submit until a difficulty is
    // chosen. Capture phase + stopPropagation beats both the native submit
    // and htmx's element listener; the post-choice requestSubmit sets
    // botChosen so the resubmission passes straight through. The
    // create-game form ("custom") is only intercepted when the computer
    // opponent is selected — human games submit untouched — and only after
    // native validation already passed (a submit event implies it did).
    document.addEventListener("submit", (e) => {
        const form = e.target.closest("form[data-bot-difficulty]");
        if (!form) return;
        if (form.dataset.botDifficulty === "custom") {
            const comp = form.querySelector("#opp-computer");
            if (!comp || !comp.checked) return;
        }
        if (form.dataset.botChosen) {
            delete form.dataset.botChosen;
            return;
        }
        e.preventDefault();
        e.stopPropagation();
        pendingForm = form;
        // the clicked color button's value would be lost to requestSubmit
        // (no submitter), so carry it via a hidden field instead
        pendingColor = e.submitter && e.submitter.name === "color" ? e.submitter.value : null;
        open();
    }, true);

    const setHidden = (form, name, value) => {
        let el = form.querySelector('input[data-bd][name="' + name + '"]');
        if (!el) {
            el = document.createElement("input");
            el.type = "hidden";
            el.name = name;
            el.dataset.bd = "1";
            form.appendChild(el);
        }
        el.value = value;
    };

    shade.querySelectorAll(".bd-card").forEach((card) =>
        card.addEventListener("click", () => {
            const form = pendingForm;
            if (!form) return;
            const key = card.dataset.bot;
            try { localStorage.setItem(STORE, key); } catch (e) {}
            setHidden(form, "bot", key);
            if (pendingColor !== null) setHidden(form, "color", pendingColor);
            close();
            form.dataset.botChosen = "1";
            form.requestSubmit();
            // the submit dispatch is synchronous, so the injected fields are
            // already serialized; drop them so a later human-opponent submit
            // of the create-game form can't carry a stale bot/color pair
            form.querySelectorAll("input[data-bd]").forEach((el) => el.remove());
        }));

    if (closeBtn) closeBtn.addEventListener("click", close);
    shade.addEventListener("click", (e) => { if (e.target === shade) close(); });
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") close(); });
})();
