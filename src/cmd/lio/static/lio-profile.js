// Player page interactions: switching which time control's rating curve is 
// shown, and the curve's hover readout.
//
// The chart itself is server-rendered inline SVG — this file never draws it.
// The server also marks one panel active, so the section is complete and
// readable with JavaScript off; everything here is enhancement on top of markup
// that already works.
(function () {
  "use strict";

  // --- refresh -------------------------------------------------------------
  //
  // The page is a snapshot of a history that keeps moving, so it reports its own
  // age and renews itself. A full reload rather than a partial swap: every
  // section here is server-rendered, so re-fetching one would mean re-running
  // every listener in this file against replaced nodes — and browsers restore
  // scroll position on reload, which is the only thing a swap would have saved.
  (function () {
    const row = document.querySelector("[data-refresh-row]");
    if (!row) return;
    const label = row.querySelector("[data-refresh-label]");
    const btn = row.querySelector("[data-refresh]");
    const rendered = Number(row.dataset.rendered);
    if (!label || !btn || !rendered) return;

    const REFRESH_MS = 10 * 60 * 1000;

    function reload() {
      btn.classList.add("is-busy");
      window.location.reload();
    }

    function age() {
      const mins = Math.floor((Date.now() - rendered) / 60000);
      if (mins < 1) return "Refreshed just now";
      if (mins < 60) return "Refreshed " + mins + "m ago";
      const hrs = Math.floor(mins / 60);
      if (hrs < 24) return "Refreshed " + hrs + "h ago";
      return "Refreshed " + Math.floor(hrs / 24) + "d ago";
    }
    function tick() {
      label.textContent = age();
    }
    tick();
    setInterval(tick, 15000);

    btn.addEventListener("click", reload);

    // Only refresh a page someone can see. A background tab reloading on a timer
    // burns a request and a database read for nobody; it is marked stale instead
    // and renews the moment it is looked at again.
    let due = false;
    setInterval(function () {
      if (document.hidden) {
        due = true;
        return;
      }
      reload();
    }, REFRESH_MS);
    document.addEventListener("visibilitychange", function () {
      if (due && !document.hidden) reload();
    });
  })();

  const section = document.getElementById("ratingHistory");
  if (!section) return;

  const panels = Array.from(section.querySelectorAll("[data-chart-panel]"));
  if (!panels.length) return;

  // Tabs live in two places — the dedicated row inside this card, and the
  // rating tiles above it, which double as the selector so a player's
  // categories and their charts are one list rather than two that can drift.
  const tabs = Array.from(document.querySelectorAll("[data-chart-tab]"));

  function activate(category) {
    panels.forEach(function (p) {
      p.classList.toggle("is-active", p.dataset.chartPanel === category);
    });
    tabs.forEach(function (t) {
      const on = t.dataset.chartTab === category;
      t.classList.toggle("is-active", on);
      // role=tab carries aria-selected; the rating tiles are plain buttons and
      // carry aria-pressed instead
      if (t.getAttribute("role") === "tab") {
        t.setAttribute("aria-selected", on ? "true" : "false");
      } else {
        t.setAttribute("aria-pressed", on ? "true" : "false");
      }
    });
  }

  tabs.forEach(function (t) {
    t.addEventListener("click", function () {
      activate(t.dataset.chartTab);
    });
  });

  // sync the controls to whichever panel the server rendered active
  const initial = panels.find(function (p) {
    return p.classList.contains("is-active");
  });
  if (initial) activate(initial.dataset.chartPanel);

  // --- hover readout ------------------------------------------------------
  //
  // The tooltip is a convenience, never the only way to read a value: the plot
  // direct-labels its endpoint and peak, the axis carries the rest, and each
  // panel ships a visually-hidden table of every point.
  panels.forEach(function (panel) {
    const plot = panel.querySelector("[data-chart-hover]");
    if (!plot) return;
    const svg = plot.querySelector("svg");
    const tip = plot.querySelector(".chart-tip");
    const cross = plot.querySelector(".chart-crosshair");
    const focus = plot.querySelector(".chart-focus");
    const data = readPoints(panel.dataset.chartPanel);
    if (!svg || !tip || !cross || !focus || data.length < 2) return;

    const box = svg.viewBox.baseVal;

    function nearest(clientX) {
      const rect = svg.getBoundingClientRect();
      if (!rect.width) return null;
      // client px -> SVG user units, the space the server emitted coordinates in
      const ux = ((clientX - rect.left) / rect.width) * box.width + box.x;
      let best = data[0];
      let bestDist = Infinity;
      for (const d of data) {
        const dist = Math.abs(d.x - ux);
        if (dist < bestDist) {
          bestDist = dist;
          best = d;
        }
      }
      return best;
    }

    // A touch reading is *pinned*: it survives the pointerleave that fires the
    // instant a tap ends, which is what made a tapped point flash and vanish.
    // A mouse reading tracks the cursor and clears when it leaves, as before.
    let pinned = false;

    function show(ev) {
      const d = nearest(ev.clientX);
      if (!d) return;
      const rect = svg.getBoundingClientRect();
      const px = ((d.x - box.x) / box.width) * rect.width;
      const py = ((d.y - box.y) / box.height) * rect.height;

      cross.setAttribute("x1", d.x);
      cross.setAttribute("x2", d.x);
      focus.setAttribute("cx", d.x);
      focus.setAttribute("cy", d.y);

      tip.innerHTML = "";
      const rating = document.createElement("span");
      rating.className = "chart-tip-rating";
      rating.textContent = d.rating + (d.provisional ? "?" : "");
      const when = document.createElement("span");
      when.className = "chart-tip-when";
      when.textContent = " " + d.when;
      tip.appendChild(rating);
      tip.appendChild(when);

      tip.style.left = px + "px";
      tip.style.top = py + "px";
      tip.hidden = false;
      plot.classList.add("is-hovering");
    }

    function hide() {
      pinned = false;
      tip.hidden = true;
      plot.classList.remove("is-hovering");
    }

    const isTouch = (ev) => ev.pointerType === "touch" || ev.pointerType === "pen";

    plot.addEventListener("pointerdown", function (ev) {
      pinned = isTouch(ev);
      show(ev);
    });
    plot.addEventListener("pointermove", function (ev) {
      // a touch drag scrubs along the curve; a mouse move just tracks
      if (!isTouch(ev) || ev.buttons || pinned) show(ev);
    });
    plot.addEventListener("pointerleave", function () {
      if (!pinned) hide();
    });
    plot.addEventListener("pointercancel", hide);

    // iOS treats a long press on an element as a selection/callout gesture,
    // which tore the readout away mid-read. The CSS kills the callout and the
    // selection; this kills the menu for the mouse case too.
    plot.addEventListener("contextmenu", function (ev) {
      ev.preventDefault();
    });

    // A pinned reading is dismissed by tapping anywhere else — the same gesture
    // that dismisses every other transient thing on the page. Capture phase so
    // it still fires when the tap lands on some other interactive element.
    document.addEventListener(
      "pointerdown",
      function (ev) {
        if (pinned && !plot.contains(ev.target)) hide();
      },
      true,
    );
    // scrolling away from a pinned point should not leave it stranded on screen
    window.addEventListener("scroll", function () {
      if (pinned) hide();
    }, { passive: true });
  });

  // --- recent-form readout ------------------------------------------------
  //
  // The strip shows bare pips and a score so it survives a narrow viewport;
  // opponent and time control live in a reserved line beneath it, which costs no
  // width and covers nothing. The score is the tap target rather than a pip:
  // pips are links to their game, and a tap that both navigates and reveals is a
  // tap that does neither well.
  (function () {
    const strip = document.querySelector(".form-strip");
    const readout = document.querySelector(".form-readout");
    if (!strip || !readout) return;

    const resultEl = readout.querySelector(".form-readout-result");
    const detailEl = readout.querySelector(".form-readout-detail");
    const latestEl = readout.querySelector(".form-readout-latest");
    if (!resultEl || !detailEl) return;

    // what the line reverts to: the server-rendered default (the newest match)
    const FALLBACK = {
      result: resultEl.textContent,
      detail: detailEl.textContent,
      cls: resultEl.className.replace("form-readout-result", "").trim(),
    };
    let pinned = null;
    let focused = null;

    // The inter-game dot is drawn in CSS on a lone game that follows another
    // lone game. Wrapping breaks that: the first group on a new row still
    // follows a lone game in source order, so it would render a dot at the row's
    // left edge separating nothing. CSS has no "first on its line" selector, so
    // the row starts are marked here and the stylesheet suppresses them.
    function markRowStarts() {
      let prevTop = null;
      for (const g of strip.querySelectorAll(".form-group")) {
        const top = g.offsetTop;
        g.classList.toggle("is-rowstart", prevTop === null || top !== prevTop);
        prevTop = top;
      }
    }
    markRowStarts();
    if (window.ResizeObserver) {
      new ResizeObserver(markRowStarts).observe(strip);
    } else {
      window.addEventListener("resize", markRowStarts);
    }

    // The nearest thing carrying a readout. A pip has its own, so pointing at
    // one game inside a match reports that game; anywhere else in the group —
    // the score, the bracket padding — falls through to the match.
    const targetOf = (el) => (el && el.closest ? el.closest("[data-form-result]") : null);

    // What gets ringed for a given readout target. Pointing at one game of a
    // match keeps the match ringed too: the game is being read *as part of* that
    // match, and dropping the match ring would make the grouping flicker away
    // exactly when someone is inspecting it. A lone game rings only itself —
    // its group's box is the pip's box, so a second ring would just double it.
    function ringsFor(el) {
      if (!el) return [];
      const group = el.closest(".form-group");
      if (group && group !== el && group.classList.contains("is-match")) {
        return [el, group];
      }
      return [el];
    }

    function paint(el) {
      for (const r of strip.querySelectorAll(".is-focus")) {
        r.classList.remove("is-focus");
      }
      for (const r of ringsFor(el)) r.classList.add("is-focus");
    }

    function write(el) {
      const src = el || FALLBACK;
      resultEl.textContent = src.result;
      resultEl.className = "form-readout-result " + src.cls;
      detailEl.textContent = src.detail;
      // the label names the white-ringed game, so it belongs only to the
      // default line — once a hover takes over, the line describes something
      // ringed in accent instead
      if (latestEl) latestEl.hidden = !!el;
    }

    const read = (el) => ({
      result: el.dataset.formResult,
      detail: el.dataset.formDetail,
      cls: el.dataset.formClass || "",
    });

    function focus(el) {
      if (focused === el) return;
      focused = el;
      paint(el);
      write(el ? read(el) : null);
    }

    function blur() {
      focused = null;
      paint(pinned);
      write(pinned ? read(pinned) : null);
    }

    function unpin() {
      pinned = null;
      focused = null;
      paint(null);
      write(null);
    }

    strip.addEventListener("pointerover", function (ev) {
      if (pinned) return;
      const el = targetOf(ev.target);
      if (el) focus(el);
    });
    strip.addEventListener("pointerleave", blur);

    // tapping a score pins its match; tapping the same one again releases it
    strip.addEventListener("pointerdown", function (ev) {
      if (!ev.target.closest("[data-form-tip]")) return;
      const el = targetOf(ev.target);
      if (!el) return;
      if (pinned === el) {
        unpin();
        return;
      }
      pinned = el;
      focus(el);
    });

    document.addEventListener(
      "pointerdown",
      function (ev) {
        if (pinned && !pinned.contains(ev.target)) unpin();
      },
      true,
    );

    // Clicking anywhere in a match opens its first game. The pips and the score
    // are real anchors and handle themselves — this only covers the container's
    // own padding, which otherwise looked clickable and did nothing.
    strip.addEventListener("click", function (ev) {
      if (ev.target.closest("a")) return;
      const group = ev.target.closest(".form-group[data-form-href]");
      if (group) window.location.href = group.dataset.formHref;
    });
  })();

  // --- activity heatmap ----------------------------------------------------
  (function () {
    const box = document.querySelector(".heatmap-scroll");
    const map = document.querySelector("[data-heatmap]");
    if (!box || !map) return;

    // The grid is a year wide. On a wide card it stretches to fill; on a phone
    // it overflows and this box scrolls. Start it at the *recent* end — left as
    // it lands it opens on last summer, and someone checking "have I played
    // this week" would have to discover a horizontal scroll to find out.
    const toEnd = () => {
      box.scrollLeft = box.scrollWidth - box.clientWidth;
    };
    toEnd();
    if (window.ResizeObserver) new ResizeObserver(toEnd).observe(box);

    // --- mode toggle: both ramps are already on every cell, so this only
    // swaps which one the stylesheet reads.
    const modes = Array.from(document.querySelectorAll("[data-heatmap-mode]"));
    const legends = Array.from(document.querySelectorAll("[data-legend]"));
    modes.forEach(function (btn) {
      btn.addEventListener("click", function () {
        const mode = btn.dataset.heatmapMode;
        map.dataset.mode = mode;
        modes.forEach((b) => b.classList.toggle("is-active", b === btn));
        legends.forEach((l) => {
          l.hidden = l.dataset.legend !== mode;
        });
      });
    });

    // --- custom tooltip, so a day can report its rate in the same tinted
    // vocabulary the rest of the page uses rather than a flat native title.
    const wrap = box.closest(".heatmap-wrap") || map;
    const tip = wrap.querySelector(".heatmap-tip");
    if (!tip) return;
    let focused = null;

    function show(cell) {
      if (focused) focused.classList.remove("is-focus");
      focused = cell;
      cell.classList.add("is-focus");

      const games = cell.dataset.games;
      const score = cell.dataset.score;
      tip.innerHTML = "";
      const main = document.createElement("span");
      main.className = "heatmap-tip-main";
      if (games) {
        main.textContent = games;
        if (score) {
          const rate = document.createElement("span");
          rate.className = rateClass(score);
          rate.textContent = " · " + score;
          main.appendChild(rate);
        }
      } else {
        main.textContent = "No games";
      }
      const when = document.createElement("span");
      when.className = "heatmap-tip-when";
      when.textContent = cell.dataset.when;
      tip.appendChild(main);
      tip.appendChild(when);

      const wr = wrap.getBoundingClientRect();
      const cr = cell.getBoundingClientRect();
      tip.hidden = false; // measurable before it is placed
      const half = tip.offsetWidth / 2;
      tip.style.left =
        Math.max(half, Math.min(cr.left - wr.left + cr.width / 2, wr.width - half)) + "px";
      tip.style.top = cr.top - wr.top - 4 + "px";
    }

    function hide() {
      if (focused) focused.classList.remove("is-focus");
      focused = null;
      tip.hidden = true;
    }

    // exactly even is neutral rather than being rounded into a win or a loss
    function rateClass(score) {
      const n = parseFloat(score);
      if (n > 0.5) return "win";
      if (n < 0.5) return "loss";
      return "draw";
    }

    map.addEventListener("pointerover", function (ev) {
      const cell = ev.target.closest(".heatmap-day");
      if (cell && !cell.classList.contains("is-future")) show(cell);
    });
    map.addEventListener("pointerleave", hide);
    box.addEventListener("scroll", hide, { passive: true });
  })();

  // --- game-length readout -------------------------------------------------
  //
  // Each column already states its dominant share on the cap; this fills in the
  // other two on demand, in win/draw/loss order — the same order the legend and
  // the stack itself use, so nothing has to be re-derived.
  (function () {
    const chart = document.querySelector(".length-chart");
    const tip = chart && chart.querySelector(".length-tip");
    if (!chart || !tip) return;

    const segs = {
      win: tip.querySelector(".length-tip-seg.win"),
      draw: tip.querySelector(".length-tip-seg.draw"),
      loss: tip.querySelector(".length-tip-seg.loss"),
    };
    if (!segs.win || !segs.draw || !segs.loss) return;

    let pinned = null;

    function show(col) {
      // a bucket with no games carries no shares and has nothing to say
      if (!col.dataset.lenWin) return;
      segs.win.textContent = "Won " + col.dataset.lenWin;
      segs.draw.textContent = "Drew " + col.dataset.lenDraw;
      segs.loss.textContent = "Lost " + col.dataset.lenLoss;

      tip.hidden = false; // measurable before it is placed

      const cr = chart.getBoundingClientRect();
      const br = col.querySelector(".length-bar-wrap").getBoundingClientRect();

      // Fixed vertically, at the top of the bar area. Following the hovered
      // bar's own cap would put the tooltip in the row of dominant-share
      // labels, covering the very numbers it elaborates on; anchoring above
      // that row would cover the card's heading instead. Here it overlays only
      // the bars, which is what a chart tooltip is expected to do.
      tip.style.top = br.top - cr.top + "px";

      // clamp so the first and last columns do not push it past the card
      const half = tip.offsetWidth / 2;
      const centre = br.left - cr.left + br.width / 2;
      tip.style.left =
        Math.max(half, Math.min(centre, cr.width - half)) + "px";
    }

    function hide() {
      pinned = null;
      tip.hidden = true;
    }

    chart.addEventListener("pointerover", function (ev) {
      if (pinned) return;
      const col = ev.target.closest(".length-col");
      if (col) show(col);
    });
    chart.addEventListener("pointerleave", function () {
      if (!pinned) tip.hidden = true;
    });

    // touch: tap pins, tapping the same column again releases
    chart.addEventListener("pointerdown", function (ev) {
      const col = ev.target.closest(".length-col");
      if (!col) return;
      if (pinned === col) {
        hide();
        return;
      }
      pinned = col;
      show(col);
    });
    document.addEventListener(
      "pointerdown",
      function (ev) {
        if (pinned && !pinned.contains(ev.target)) hide();
      },
      true,
    );
  })();

  // readPoints pulls one panel's samples out of the inline JSON the server
  // emitted beside it. The coordinates are the same ones the SVG was drawn
  // with, so the hover layer cannot disagree with the rendered line.
  function readPoints(category) {
    const el = document.getElementById("chartData-" + category);
    if (!el) return [];
    try {
      return (JSON.parse(el.textContent) || []).map(function (d) {
        return {
          x: parseFloat(d.X),
          y: parseFloat(d.Y),
          rating: d.Rating,
          when: d.When,
          provisional: !!d.Provisional,
        };
      });
    } catch (e) {
      return [];
    }
  }
})();
