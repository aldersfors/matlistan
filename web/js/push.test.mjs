// Tests for the pure helpers in internal/web/static/push.js.
// Run with: node --test web/js/
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const _source = readFileSync(new URL("../../internal/web/static/push.js", import.meta.url), "utf8");
const ctx = { document: { addEventListener() {} }, window: {}, navigator: {}, btoa, atob };
vm.runInNewContext(_source, ctx);

// Review focus 2: an iPhone Safari tab gets the Home Screen hint, not a broken button.
test("pushState", () => {
  assert.equal(ctx.pushState({ hasWorker: true, hasPush: false, ios: true, standalone: false, permission: "default" }), "ios-home-screen");
  assert.equal(ctx.pushState({ hasWorker: false, hasPush: false, ios: false, standalone: false, permission: "default" }), "unsupported");
  assert.equal(ctx.pushState({ hasWorker: true, hasPush: true, ios: false, standalone: false, permission: "denied" }), "denied");
  assert.equal(ctx.pushState({ hasWorker: true, hasPush: true, ios: true, standalone: true, permission: "default" }), "ready");
});

// Review focus 1: a subscription made with an old VAPID key is replaced.
test("subscriptionMatches", () => {
  const key = new Uint8Array([4, 1, 2, 3]).buffer;
  assert.equal(ctx.subscriptionMatches(key, "BAECAw"), true);
  assert.equal(ctx.subscriptionMatches(key, "BAECBA"), false);
  assert.equal(ctx.subscriptionMatches(null, "BAECAw"), false);
});
