// Full-page screenshots at a real 390px phone viewport through the Chrome DevTools protocol.
// Headless Chrome on macOS will not open a window narrower than 500px, so --window-size
// cannot emulate a phone; device metrics emulation can.
// Usage: node hack/shoot.mjs <out-dir> <base-url> <light|dark> name=path...
// SHOOT_FRAME=1 captures one phone screen at 2x instead of the full page, as the README shows
// them: a full page puts the fixed tab bar in the middle of a long screen. SHOOT_SIZE=WxH
// sets another viewport (the README banner is 1280x640).
import { spawn } from "node:child_process";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const [out, base, scheme, ...shots] = process.argv.slice(2);
const frame = process.env.SHOOT_FRAME === "1";
const [vw, vh] = (process.env.SHOOT_SIZE || "390x844").split("x").map(Number);
const chrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const profile = mkdtempSync(join(tmpdir(), "matlistan-shoot-"));
const port = 9333;
const proc = spawn(chrome, ["--headless=new", "--disable-gpu", "--hide-scrollbars",
  `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, "about:blank"],
  { stdio: "ignore" });

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function pageSocket() {
  for (let i = 0; i < 50; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
      const page = list.find((t) => t.type === "page");
      if (page) return page.webSocketDebuggerUrl;
    } catch { /* not up yet */ }
    await sleep(100);
  }
  throw new Error("chrome did not start");
}

const ws = new WebSocket(await pageSocket());
await new Promise((r) => ws.addEventListener("open", r, { once: true }));
let seq = 0;
const pending = new Map();
const waiters = [];
ws.addEventListener("message", (e) => {
  const msg = JSON.parse(e.data);
  if (msg.id && pending.has(msg.id)) {
    const { resolve, reject } = pending.get(msg.id);
    pending.delete(msg.id);
    msg.error ? reject(new Error(msg.error.message)) : resolve(msg.result);
  } else if (msg.method) {
    for (const w of waiters.filter((w) => w.method === msg.method)) {
      waiters.splice(waiters.indexOf(w), 1);
      w.resolve(msg.params);
    }
  }
});
const send = (method, params = {}) => new Promise((resolve, reject) => {
  const id = ++seq;
  pending.set(id, { resolve, reject });
  ws.send(JSON.stringify({ id, method, params }));
});
const next = (method) => new Promise((resolve) => waiters.push({ method, resolve }));

await send("Page.enable");
await send("Emulation.setDeviceMetricsOverride",
  { width: vw, height: vh, deviceScaleFactor: frame ? 2 : 1, mobile: vw < 500 });
await send("Emulation.setEmulatedMedia",
  { features: [{ name: "prefers-color-scheme", value: scheme }] });

try {
  for (const shot of shots) {
    // name=path, or name=path!js to run a script (e.g. clicks) before the capture.
    const [name, rest] = shot.split(/=(.*)/s);
    const [path, script] = rest.split(/!(.*)/s);
    const loaded = next("Page.loadEventFired");
    await send("Page.navigate", { url: base + path });
    await loaded;
    await sleep(400); // redirects through the dev IdP land here too; let them settle
    if (script) {
      await send("Runtime.evaluate", { expression: script });
      await sleep(200);
    }
    const { cssContentSize } = await send("Page.getLayoutMetrics");
    const height = frame ? vh : Math.max(vh, Math.ceil(cssContentSize.height));
    const { data } = await send("Page.captureScreenshot", { format: "png",
      captureBeyondViewport: true, clip: { x: 0, y: 0, width: vw, height, scale: 1 } });
    writeFileSync(join(out, `${name}-${scheme}.png`), Buffer.from(data, "base64"));
    const { result } = await send("Runtime.evaluate",
      { expression: "document.documentElement.scrollWidth" });
    if (result.value > vw) console.log(`${name}-${scheme}: page is ${result.value}px wide`);
  }
} finally {
  ws.close();
  proc.kill();
  await sleep(200);
  rmSync(profile, { recursive: true, force: true });
}
