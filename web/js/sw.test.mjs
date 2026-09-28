// Tests for internal/web/static/sw.js with a fake service worker global.
// Run with: node --test web/js/
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const _source = readFileSync(new URL("../../internal/web/static/sw.js", import.meta.url), "utf8");

function load() {
  const handlers = {};
  const shown = [];
  const opened = [];
  const windows = [];
  const self = {
    location: { origin: "https://matlistan.example.org" },
    addEventListener: (type, fn) => { handlers[type] = fn; },
    registration: { showNotification: async (title, opts) => { shown.push({ title, opts }); } },
    clients: {
      matchAll: async () => windows,
      openWindow: async (url) => { opened.push(url); },
    },
  };
  vm.runInNewContext(_source, { self, URL });
  return { handlers, shown, opened, windows };
}

function event(data) {
  let waited;
  return { data: { json: () => data }, waitUntil: (p) => { waited = p; }, done: () => waited };
}

test("push shows the notification with its URL", async () => {
  const sw = load();
  const e = event({ title: "Veckans förslag är klart", body: "Vecka 41 väntar på ditt godkännande.", url: "/week?y=2026&w=41" });
  sw.handlers.push(e);
  await e.done();
  assert.equal(sw.shown[0].title, "Veckans förslag är klart");
  assert.equal(sw.shown[0].opts.data.url, "/week?y=2026&w=41");
});

test("a click opens the week on this site", async () => {
  const sw = load();
  let closed = false;
  const e = { notification: { data: { url: "/week?y=2026&w=41" }, close: () => { closed = true; } }, waitUntil(p) { this.p = p; } };
  sw.handlers.notificationclick(e);
  await e.p;
  assert.ok(closed);
  assert.deepEqual(sw.opened, ["https://matlistan.example.org/week?y=2026&w=41"]);
});

test("a click never leaves the site", async () => {
  for (const url of ["https://evil.example/", "//evil.example/x", "javascript:alert(1)", "week"]) {
    const sw = load();
    const e = { notification: { data: { url }, close() {} }, waitUntil(p) { this.p = p; } };
    sw.handlers.notificationclick(e);
    await e.p;
    assert.deepEqual(sw.opened, ["https://matlistan.example.org/"], url);
  }
});

test("a click focuses an open window instead of opening another", async () => {
  const sw = load();
  const nav = [];
  sw.windows.push({ url: "https://matlistan.example.org/shopping", focus: async function () { return this; }, navigate: async (u) => { nav.push(u); } });
  const e = { notification: { data: { url: "/week?y=2026&w=41" }, close() {} }, waitUntil(p) { this.p = p; } };
  sw.handlers.notificationclick(e);
  await e.p;
  assert.deepEqual(sw.opened, []);
  assert.deepEqual(nav, ["https://matlistan.example.org/week?y=2026&w=41"]);
});
