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

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = sameSite(event.notification.data && event.notification.data.url);
  event.waitUntil((async () => {
    const wins = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    for (const w of wins) {
      if (new URL(w.url).origin === self.location.origin) {
        const f = await w.focus();
        return (f || w).navigate(url);
      }
    }
    return self.clients.openWindow(url);
  })());
});
