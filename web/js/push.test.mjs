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

// A fake PushManager with one optional existing subscription.
function fakeReg({ existingKey = null, subscribeThrows = false } = {}) {
  const calls = { unsubscribed: 0, subscribed: 0 };
  const mkSub = (keyBytes, endpoint) => ({
    endpoint, options: { applicationServerKey: keyBytes ? new Uint8Array(keyBytes).buffer : null },
    toJSON: () => ({ endpoint, keys: { p256dh: "p", auth: "a" } }),
    unsubscribe: async () => { calls.unsubscribed++; return true; },
  });
  const reg = {
    pushManager: {
      getSubscription: async () => existingKey ? mkSub(existingKey, "https://web.push.apple.com/old") : null,
      subscribe: async () => {
        if (subscribeThrows) throw new Error("AbortError");
        calls.subscribed++;
        return mkSub([4, 1, 2, 3], "https://web.push.apple.com/new");
      },
    },
  };
  return { reg, calls };
}

function recorder(ok = true) {
  const posts = [];
  const post = async (url, body) => { posts.push({ url, endpoint: body.endpoint }); return ok; };
  return { posts, post };
}

// Review focus 1: a subscription made with an old key is replaced, not just turned off.
test("syncOnLoad re-subscribes after a key change", async () => {
  const { reg, calls } = fakeReg({ existingKey: [4, 9, 9, 9] });
  const { posts, post } = recorder();
  const state = await ctx.syncOnLoad(reg, "BAECAw", "granted", post);
  assert.equal(state, "on");
  assert.equal(calls.unsubscribed, 1);
  assert.equal(calls.subscribed, 1);
  assert.deepEqual(posts.map((p) => p.url), ["/settings/push/delete", "/settings/push"]);
  assert.equal(posts[1].endpoint, "https://web.push.apple.com/new");
});

test("syncOnLoad falls back to off when the browser needs a tap to subscribe", async () => {
  const { reg } = fakeReg({ existingKey: [4, 9, 9, 9], subscribeThrows: true });
  assert.equal(await ctx.syncOnLoad(reg, "BAECAw", "granted", recorder().post), "off");
});

// The server row may be gone (a failed save, a restore): the page re-saves before saying on.
test("syncOnLoad re-saves a matching subscription", async () => {
  const { reg } = fakeReg({ existingKey: [4, 1, 2, 3] });
  const ok = recorder(true);
  assert.equal(await ctx.syncOnLoad(reg, "BAECAw", "granted", ok.post), "on");
  assert.deepEqual(ok.posts.map((p) => p.url), ["/settings/push"]);
  const bad = recorder(false);
  assert.equal(await ctx.syncOnLoad(reg, "BAECAw", "granted", bad.post), "failed");
});

test("syncOnLoad without a subscription is off and posts nothing", async () => {
  const { reg } = fakeReg();
  const { posts, post } = recorder();
  assert.equal(await ctx.syncOnLoad(reg, "BAECAw", "default", post), "off");
  assert.equal(posts.length, 0);
});

test("turnOn reports denial, a failed subscribe and success", async () => {
  assert.equal(await ctx.turnOn(fakeReg().reg, "BAECAw", async () => "denied", recorder().post), "denied");
  assert.equal(await ctx.turnOn(fakeReg({ subscribeThrows: true }).reg, "BAECAw", async () => "granted", recorder().post), "failed");
  assert.equal(await ctx.turnOn(fakeReg().reg, "BAECAw", async () => "granted", recorder().post), "on");
  assert.equal(await ctx.turnOn(fakeReg().reg, "BAECAw", async () => "granted", recorder(false).post), "failed");
});
