// Tests for internal/web/static/copy.js with a fake DOM and clipboard.
// Run with: node --test web/js/
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const _source = readFileSync(new URL("../../internal/web/static/copy.js", import.meta.url), "utf8");

test("a copy button copies its target and says so", async () => {
  let handler;
  const written = [];
  const pre = { textContent: "- key: anna\n" };
  const button = { dataset: { copy: "yaml-member-1", done: "Kopierat" }, textContent: "Kopiera",
    closest(sel) { return sel === "[data-copy]" ? this : null; } };
  const ctx = {
    document: { addEventListener: (type, fn) => { if (type === "click") handler = fn; },
      getElementById: (id) => (id === "yaml-member-1" ? pre : null) },
    navigator: { clipboard: { writeText: async (s) => { written.push(s); } } },
  };
  vm.runInNewContext(_source, ctx);
  await handler({ target: button });
  assert.deepEqual(written, ["- key: anna\n"]);
  assert.equal(button.textContent, "Kopierat");
});
