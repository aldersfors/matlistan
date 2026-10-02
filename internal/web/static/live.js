// Keeps an open week page current: when anyone changes the week, <main> is loaded again.
"use strict";

(() => {
  const main = document.querySelector("main[data-live]");
  if (!main || typeof EventSource === "undefined") return;
  const url = main.dataset.live;
  let source = null;
  let opened = false;
  let waiting = false; // a refresh waits for a field to be left

  // Text being typed is not replaced; the refresh waits until the field is left, when its
  // own change has saved it.
  const typing = () => {
    const el = document.activeElement;
    return el && el.closest && el.closest("main") &&
      (el.tagName === "TEXTAREA" || (el.tagName === "INPUT" && el.type === "text")) ? el : null;
  };

  const refresh = () => {
    const field = typing();
    if (field) {
      if (!waiting) {
        waiting = true;
        field.addEventListener("blur", () => { waiting = false; refresh(); }, { once: true });
      }
      return;
    }
    const details = document.querySelector("main details");
    const open = Boolean(details && details.open);
    htmx.ajax("GET", location.href, { target: "main", select: "main", swap: "outerHTML" })
      .then(() => {
        const next = document.querySelector("main details");
        if (next && open) next.open = true;
      });
  };

  const connect = () => {
    source = new EventSource(url);
    source.addEventListener("week", refresh);
    // A reconnect may have missed changes, so it catches up once.
    source.addEventListener("open", () => {
      if (opened) refresh();
      opened = true;
    });
  };

  window.addEventListener("pagehide", () => {
    if (source) source.close();
    source = null;
  });
  window.addEventListener("pageshow", (event) => {
    if (event.persisted && !source) connect(); // its open event catches up
  });
  connect();
})();
