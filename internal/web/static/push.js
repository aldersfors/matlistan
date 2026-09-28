// Turns web push on or off for this device from the Settings page.
"use strict";

function b64url(buf) {
  let s = "";
  for (const b of new Uint8Array(buf)) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromB64url(s) {
  const b = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(b, (c) => c.charCodeAt(0));
}

// pushState is what the section can offer on this device.
function pushState(env) {
  if (env.ios && !env.standalone && !env.hasPush) return "ios-home-screen";
  if (!env.hasWorker || !env.hasPush) return "unsupported";
  if (env.permission === "denied") return "denied";
  return "ready";
}

// subscriptionMatches is true when the existing subscription used this server's key.
function subscriptionMatches(appServerKey, pageKey) {
  return !!appServerKey && b64url(appServerKey) === pageKey;
}

async function post(url, body) {
  const res = await fetch(url, { method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body), credentials: "same-origin" });
  return res.ok;
}

function subscribe(reg, key) {
  return reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: fromB64url(key) });
}

// syncOnLoad brings this device in line with the server when Settings opens. A
// subscription made with an old key is replaced, and a matching one is saved again, since
// the server's row may be gone (a failed save, a restore). "on" only after the server has it.
async function syncOnLoad(reg, key, permission, post) {
  let sub = await reg.pushManager.getSubscription();
  if (sub && !subscriptionMatches(sub.options.applicationServerKey, key)) {
    await post("/settings/push/delete", { endpoint: sub.endpoint });
    await sub.unsubscribe();
    if (permission !== "granted") return "off";
    try {
      sub = await subscribe(reg, key);
    } catch {
      return "off"; // some browsers only subscribe after a tap
    }
  }
  if (!sub) return "off";
  return (await post("/settings/push", sub.toJSON())) ? "on" : "failed";
}

async function turnOn(reg, key, requestPermission, post) {
  if (await requestPermission() !== "granted") return "denied";
  try {
    const sub = await subscribe(reg, key);
    return (await post("/settings/push", sub.toJSON())) ? "on" : "failed";
  } catch {
    return "failed";
  }
}

async function turnOff(reg, post) {
  const sub = await reg.pushManager.getSubscription();
  if (sub) {
    await post("/settings/push/delete", { endpoint: sub.endpoint });
    await sub.unsubscribe();
  }
  return "off";
}

document.addEventListener("DOMContentLoaded", async () => {
  const root = document.getElementById("push");
  if (!root) return;
  const key = root.dataset.key;
  const show = (state) => { root.dataset.state = state; };
  const env = {
    hasWorker: "serviceWorker" in navigator, hasPush: "PushManager" in window,
    ios: /iP(hone|ad|od)/.test(navigator.userAgent),
    standalone: window.matchMedia && window.matchMedia("(display-mode: standalone)").matches,
    permission: typeof Notification === "undefined" ? "default" : Notification.permission,
  };
  const state = pushState(env);
  if (state !== "ready") { show(state); return; }
  let reg;
  try {
    await navigator.serviceWorker.register("/sw.js", { scope: "/" });
    reg = await navigator.serviceWorker.ready; // subscribe needs an active worker
    show(await syncOnLoad(reg, key, env.permission, post));
  } catch {
    show("failed");
  }
  const run = (fn) => async () => {
    try {
      if (!reg) reg = await navigator.serviceWorker.ready;
      show(await fn());
    } catch {
      show("failed");
    }
  };
  root.querySelector("[data-push-on]").addEventListener("click",
    run(() => turnOn(reg, key, () => Notification.requestPermission(), post)));
  root.querySelector("[data-push-off]").addEventListener("click", run(() => turnOff(reg, post)));
});
