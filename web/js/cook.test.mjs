// Tests for internal/web/static/cook.js against a small fake DOM and a mocked clock.
// Run with: node --test web/js/
import { test, mock } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const _source = readFileSync(new URL("../../internal/web/static/cook.js", import.meta.url), "utf8");

class El {
  constructor(id = "", dataset = {}) {
    this.id = id;
    this.dataset = { ...dataset };
    this.hidden = false;
    this.disabled = false;
    this.textContent = "";
    this.children = [];
    this.listeners = {};
    this.attrs = {};
  }
  append(...els) { this.children.push(...els); return this; }
  appendChild(el) { this.children.push(el); return el; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  click() { if (!this.disabled) (this.listeners.click || []).forEach((fn) => fn()); }
  focus() { focused = this; }
  descendants() { return this.children.flatMap((c) => [c, ...c.descendants()]); }
  querySelectorAll(sel) {
    const key = sel.match(/^\[data-([a-z-]+)\]$/)[1].replace(/-([a-z])/g, (_, c) => c.toUpperCase());
    return this.descendants().filter((e) => key in e.dataset);
  }
}

let focused = null;

// page builds the markup cook.templ renders for a four-step recipe with a 10-minute timer
// on step 3.
function page() {
  const steps = [1, 2, 3, 4].map(() => new El("", { step: "" }));
  const timer = new El("", { minutes: "10", done: "Klart!" });
  timer.hidden = true;
  timer.textContent = "Starta timer 10 minuter";
  steps[2].append(timer);
  const els = {
    cook: new El("cook"),
    prev: new El("cook-prev"),
    next: new El("cook-next"),
    progress: new El("cook-progress", { template: "Steg {n} av {total}" }),
    timers: new El("cook-timers", { template: "Steg {n}: {left}" }),
    announce: new El("cook-announce"),
  };
  for (const k of ["prev", "next", "progress", "timers"]) els[k].hidden = true;
  els.cook.append(els.progress, els.timers, els.announce, ...steps, els.prev, els.next);
  const byId = Object.fromEntries(Object.values(els).map((e) => [e.id, e]));
  const document = {
    visibilityState: "visible",
    getElementById: (id) => byId[id] || null,
    createElement: () => new El(),
    addEventListener() {},
  };
  const audio = { created: 0, beeps: 0 };
  class AudioContext {
    constructor() { audio.created += 1; this.currentTime = 0; this.destination = {}; }
    resume() { return Promise.resolve(); }
    createOscillator() {
      return { frequency: { value: 0 }, connect() {}, start() { audio.beeps += 1; }, stop() {} };
    }
    createGain() {
      return { gain: { value: 0, setValueAtTime() {}, exponentialRampToValueAtTime() {} },
        connect() {} };
    }
  }
  vm.runInNewContext(_source, { document, navigator: {}, window: { AudioContext },
    AudioContext, setInterval, clearInterval, Date, Number, String, Math, Array });
  return { ...els, steps, timer, audio };
}

function withClock(fn) {
  mock.timers.enable({ apis: ["setInterval", "Date"], now: 1_000_000 });
  try { fn(); } finally { mock.timers.reset(); }
}

test("a running timer stays visible on every step", () => withClock(() => {
  const p = page();
  p.next.click();
  p.next.click();
  p.timer.click();
  assert.equal(p.timers.hidden, false);
  assert.equal(p.timers.children.length, 1);
  assert.equal(p.timers.children[0].textContent, "Steg 3: 10:00");
  p.prev.click();
  assert.equal(p.steps[2].hidden, true, "step 3 is hidden on step 2");
  assert.equal(p.timers.hidden, false, "the timer strip is not inside a step");
}));

test("the countdown follows the clock, not the number of ticks", () => withClock(() => {
  const p = page();
  p.next.click();
  p.next.click();
  p.timer.click();
  // A throttled tab: five minutes pass while no interval fires.
  mock.timers.setTime(1_000_000 + 5 * 60_000);
  mock.timers.tick(1000);
  assert.equal(p.timers.children[0].textContent, "Steg 3: 4:59");
}));

test("a finished timer is announced and heard", () => withClock(() => {
  const p = page();
  p.next.click();
  p.next.click();
  p.timer.click();
  assert.equal(p.audio.created, 1, "the sound is unlocked by the tap that starts the timer");
  mock.timers.tick(10 * 60_000 + 1000);
  assert.equal(p.timers.children[0].textContent, "Steg 3: Klart!");
  assert.equal(p.announce.textContent, "Steg 3: Klart!");
  assert.ok(p.audio.beeps > 0, "no sound on finish");
  assert.equal(p.timer.disabled, false);
}));

test("starting the same timer again replaces its line", () => withClock(() => {
  const p = page();
  p.next.click();
  p.next.click();
  p.timer.click();
  mock.timers.tick(10 * 60_000 + 1000);
  p.timer.click();
  assert.equal(p.timers.children.filter((c) => !c.hidden).length, 1);
}));
