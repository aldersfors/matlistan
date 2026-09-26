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

  // Running timers live in a strip outside the steps, so they stay visible on every step.
  // Each counts down to a deadline, not in ticks: a throttled or backgrounded tab still
  // shows the right time when it wakes.
  const strip = document.getElementById("cook-timers");
  const announce = document.getElementById("cook-announce");
  const lines = new Map();
  let audio = null;

  function clock(seconds) {
    return Math.floor(seconds / 60) + ":" + String(seconds % 60).padStart(2, "0");
  }

  function beep() {
    if (!audio) return;
    for (let i = 0; i < 3; i++) {
      const at = audio.currentTime + i * 0.5;
      const osc = audio.createOscillator();
      const gain = audio.createGain();
      osc.frequency.value = 880;
      gain.gain.setValueAtTime(0.3, at);
      gain.gain.exponentialRampToValueAtTime(0.001, at + 0.4);
      osc.connect(gain);
      gain.connect(audio.destination);
      osc.start(at);
      osc.stop(at + 0.4);
    }
  }

  function startTimer(button, stepNumber) {
    // iOS only plays sound from a context created or resumed inside a tap.
    const Ctx = window.AudioContext || window.webkitAudioContext;
    if (!audio && Ctx) audio = new Ctx();
    if (audio) audio.resume();
    const deadline = Date.now() + Number(button.dataset.minutes) * 60000;
    let line = lines.get(button);
    if (!line) {
      line = document.createElement("li");
      strip.appendChild(line);
      lines.set(button, line);
    }
    line.hidden = false;
    strip.hidden = false;
    button.disabled = true;
    const text = function (left) {
      return strip.dataset.template.replace("{n}", String(stepNumber)).replace("{left}", left);
    };
    const tick = function () {
      const left = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
      line.textContent = text(clock(left));
      button.textContent = clock(left);
      if (left > 0) return;
      clearInterval(timer);
      line.textContent = text(button.dataset.done);
      button.textContent = button.dataset.done;
      announce.textContent = line.textContent;
      button.disabled = false;
      beep();
      if (navigator.vibrate) navigator.vibrate([300, 150, 300]);
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
  steps.forEach(function (step, n) {
    step.querySelectorAll("[data-minutes]").forEach(function (b) {
      b.hidden = false;
      b.addEventListener("click", function () { startTimer(b, n + 1); });
    });
  });
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "visible") keepAwake();
  });
  keepAwake();
})();
