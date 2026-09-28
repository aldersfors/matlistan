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
  const fetched = [];
  const self = {
    location: { origin: "https://matlistan.example.org" },
    addEventListener: (type, fn) => { handlers[type] = fn; },
    registration: {
      showNotification: async (title, opts) => { shown.push({ title, opts }); },
      pushManager: { subscribe: async () => ({ toJSON: () => ({ endpoint: "https://web.push.apple.com/rotated", keys: {} }) }) },
    },
    clients: {
      matchAll: async () => windows,
      openWindow: async (url) => { opened.push(url); },
      claim: async () => {},
    },
  };
  const fetch = async (url, opts) => { fetched.push({ url, opts }); return { ok: true }; };
  vm.runInNewContext(_source, { self, URL, fetch });
  return { handlers, shown, opened, windows, fetched, self };
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

test("the worker takes control of open pages when it activates", async () => {
  const sw = load();
  let claimed = false;
  sw.self.clients.claim = async () => { claimed = true; };
  const e = { waitUntil(p) { this.p = p; } };
  sw.handlers.activate(e);
  await e.p;
  assert.ok(claimed);
});

// An uncontrolled window cannot be navigated: open the week in a new window instead.
test("a click still opens the week when navigate fails", async () => {
  const sw = load();
  sw.windows.push({ url: "https://matlistan.example.org/settings", focus: async function () { return this; },
    navigate: async () => { throw new TypeError("not controlled"); } });
  const e = { notification: { data: { url: "/week?y=2026&w=41" }, close() {} }, waitUntil(p) { this.p = p; } };
  sw.handlers.notificationclick(e);
  await e.p;
  assert.deepEqual(sw.opened, ["https://matlistan.example.org/week?y=2026&w=41"]);
});

// A browser that rotates the subscription gets the new one saved on the server.
test("pushsubscriptionchange subscribes again and saves it", async () => {
  const sw = load();
  const e = { oldSubscription: { options: { applicationServerKey: new Uint8Array([4, 1]).buffer } }, waitUntil(p) { this.p = p; } };
  sw.handlers.pushsubscriptionchange(e);
  await e.p;
  assert.equal(sw.fetched.length, 1);
  assert.equal(sw.fetched[0].url, "/settings/push");
  assert.equal(JSON.parse(sw.fetched[0].opts.body).endpoint, "https://web.push.apple.com/rotated");
});
