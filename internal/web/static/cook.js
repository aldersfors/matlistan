// Cook mode: one step at a time, timers, and a screen that stays on. Without this script
// the page shows every step, which is enough to cook from.
(function () {
  "use strict";
  const root = document.getElementById("cook");
  if (!root) return;
  const steps = Array.from(root.querySelectorAll("[data-step]"));
  const prev = document.getElementById("cook-prev");
  const next = document.getElementById("cook-next");
  const progress = document.getElementById("cook-progress");
  let current = 0;

  function show() {
    steps.forEach(function (s, n) { s.hidden = n !== current; });
    progress.textContent = progress.dataset.template
      .replace("{n}", String(current + 1)).replace("{total}", String(steps.length));
    prev.disabled = current === 0;
    next.disabled = current === steps.length - 1;
  }

  function startTimer(button) {
    let left = Number(button.dataset.minutes) * 60;
    button.disabled = true;
    const tick = function () {
      const m = Math.floor(left / 60);
      const s = String(left % 60).padStart(2, "0");
      button.textContent = m + ":" + s;
      if (left <= 0) {
        clearInterval(timer);
        button.textContent = button.dataset.done;
        button.disabled = false;
        if (navigator.vibrate) navigator.vibrate([300, 150, 300]);
      }
      left -= 1;
    };
    const timer = setInterval(tick, 1000);
    tick();
  }

  async function keepAwake() {
    try {
      if ("wakeLock" in navigator) await navigator.wakeLock.request("screen");
    } catch (e) {
      // The browser refused (battery saver, no permission); cooking still works.
    }
  }

  if (steps.length > 1) {
    prev.hidden = false;
    next.hidden = false;
    progress.hidden = false;
    prev.addEventListener("click", function () { if (current > 0) { current -= 1; show(); } });
    next.addEventListener("click", function () {
      if (current < steps.length - 1) { current += 1; show(); }
    });
    show();
  }
  root.querySelectorAll("[data-minutes]").forEach(function (b) {
    b.hidden = false;
    b.addEventListener("click", function () { startTimer(b); });
  });
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "visible") keepAwake();
  });
  keepAwake();
})();
