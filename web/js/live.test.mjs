// Tests for internal/web/static/live.js with a fake DOM, EventSource and htmx.
// Run with: node --test web/js/
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const _source = readFileSync(new URL("../../internal/web/static/live.js", import.meta.url), "utf8");

class Target {
  constructor() { this.listeners = {}; }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  emit(type, event = {}) {
    const fns = this.listeners[type] || [];
    this.listeners[type] = fns.filter((fn) => !fn.once);
    fns.forEach((fn) => fn(event));
  }
}

// page loads live.js against a week page; field is what has focus, if anything.
function page() {
  const main = { dataset: { live: "/week/events?y=2026&w=40" } };
  const details = { open: true };
  const sources = [];
  const loads = [];
  const win = new Target();
  const doc = {
    activeElement: null,
    querySelector: (sel) => ({ "main[data-live]": main, "main details": doc.details })[sel] ?? null,
    details,
  };
  class FakeEventSource extends Target {
    constructor(url) { super(); this.url = url; this.closed = false; sources.push(this); }
    close() { this.closed = true; }
  }
  const htmx = {
    ajax(method, url, opts) {
      loads.push({ method, url, ...opts });
      doc.details = { open: false }; // the swap brings a closed <details>
      return Promise.resolve();
    },
  };
  vm.runInNewContext(_source, { document: doc, window: win, EventSource: FakeEventSource, htmx,
    location: { href: "/week?y=2026&w=40" } });
  return { doc, sources, loads, win };
}

function field(tag, type) {
  const f = new Target();
  f.tagName = tag;
  f.type = type;
  f.closest = (sel) => (sel === "main" ? {} : null);
  const add = f.addEventListener.bind(f);
  f.addEventListener = (t, fn, o) => { if (o?.once) fn.once = true; add(t, fn); };
  return f;
}

test("a week event loads <main> again and keeps the conditions open", async () => {
  const { doc, sources, loads } = page();
  assert.equal(sources.length, 1);
  assert.equal(sources[0].url, "/week/events?y=2026&w=40");
  sources[0].emit("week");
  assert.deepEqual(loads, [{ method: "GET", url: "/week?y=2026&w=40", target: "main",
    select: "main", swap: "outerHTML" }]);
  await Promise.resolve();
  assert.equal(doc.details.open, true);
});

test("text being typed is not replaced until the field is left, once", () => {
  const { doc, sources, loads } = page();
  const area = field("TEXTAREA");
  doc.activeElement = area;
  sources[0].emit("week");
  sources[0].emit("week");
  assert.equal(loads.length, 0);
  doc.activeElement = null;
  area.emit("blur");
  assert.equal(loads.length, 1);
});

test("a checkbox with focus does not hold the refresh back", () => {
  const { doc, sources, loads } = page();
  doc.activeElement = field("INPUT", "checkbox");
  sources[0].emit("week");
  assert.equal(loads.length, 1);
});

test("a reconnect catches up, the first connect does not", () => {
  const { sources, loads } = page();
  sources[0].emit("open");
  assert.equal(loads.length, 0);
  sources[0].emit("open");
  assert.equal(loads.length, 1);
});

test("leaving the page closes the stream; coming back from the cache reopens it", () => {
  const { sources, loads, win } = page();
  sources[0].emit("open");
  win.emit("pagehide");
  assert.equal(sources[0].closed, true);
  win.emit("pageshow", { persisted: true });
  assert.equal(sources.length, 2);
  sources[1].emit("open");
  assert.equal(loads.length, 1);
});
