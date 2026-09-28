// Matlistan service worker: shows web push notifications and opens the page they name.
// It has no fetch handler and caches nothing, so the app works exactly as without it.
"use strict";

function sameSite(path) {
  const origin = self.location.origin;
  if (typeof path !== "string" || !path.startsWith("/") || path.startsWith("//")) return origin + "/";
  const u = new URL(path, origin);
  return u.origin === origin ? u.href : origin + "/";
}

self.addEventListener("push", (event) => {
  let m = {};
  try { m = event.data ? event.data.json() : {}; } catch { m = {}; }
  event.waitUntil(self.registration.showNotification(m.title || "Matlistan", {
    body: m.body || "", icon: "/static/icons/icon-192.png", data: { url: m.url || "/" },
  }));
});

// Take control of the page that registered the worker, so the first notification's tap can
// navigate it.
self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

// A browser that rotates the subscription gives us a new one to save; without this the
// server keeps the old endpoint, which then answers 410 and is deleted.
self.addEventListener("pushsubscriptionchange", (event) => {
  const old = event.oldSubscription;
  event.waitUntil((async () => {
    const sub = await self.registration.pushManager.subscribe({ userVisibleOnly: true,
      applicationServerKey: old && old.options.applicationServerKey });
    await fetch("/settings/push", { method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json" }, body: JSON.stringify(sub.toJSON()) });
  })());
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = sameSite(event.notification.data && event.notification.data.url);
  event.waitUntil((async () => {
    const wins = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    for (const w of wins) {
      if (new URL(w.url).origin === self.location.origin) {
        try {
          const f = await w.focus();
          return await (f || w).navigate(url);
        } catch {
          break; // an uncontrolled window cannot be navigated: open a new one
        }
      }
    }
    return self.clients.openWindow(url);
  })());
});
