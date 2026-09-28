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
  const reg = await navigator.serviceWorker.register("/sw.js", { scope: "/" });
  let sub = await reg.pushManager.getSubscription();
  if (sub && !subscriptionMatches(sub.options.applicationServerKey, key)) {
    await post("/settings/push/delete", { endpoint: sub.endpoint });
    await sub.unsubscribe();
    sub = null;
    show("off");
  }
  show(sub ? "on" : "off");
  root.querySelector("[data-push-on]").addEventListener("click", async () => {
    if (await Notification.requestPermission() !== "granted") { show("denied"); return; }
    const s = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: fromB64url(key) });
    show(await post("/settings/push", s.toJSON()) ? "on" : "failed");
  });
  root.querySelector("[data-push-off]").addEventListener("click", async () => {
    const s = await reg.pushManager.getSubscription();
    if (s) { await post("/settings/push/delete", { endpoint: s.endpoint }); await s.unsubscribe(); }
    show("off");
  });
});
