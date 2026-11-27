import assert from "node:assert/strict";
import { test } from "node:test";
import { execFile } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";
import { build } from "vite";

const require = createRequire(import.meta.url);
const root = fileURLToPath(new URL("..", import.meta.url));
const browser = process.env.BROWSER_BIN ?? (process.platform === "win32"
  ? "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" : "");

// Runs the actual extension and xterm inside the session's flex/grid hierarchy.
async function checkLayout() {
  const id = "a".repeat(64);
  const resizes = [];
  let execStarts = 0;
  window.go = { main: { App: {
    GetDockerOverview: async () => ({ status: "ready", version: "27", name: "test-server", warning: "", containers: [
      { id, name: "api", state: "running", status: "Up 3 hours", ports: "8080/tcp" },
    ] }),
    GetDockerDetails: async () => ({ id, created: "2026-11-20T10:00:00Z", started: "2026-11-27T07:30:00Z",
      finished: "", exitCode: 0, restartCount: 0, restartPolicy: "unless-stopped",
      mounts: Array.from({ length: 25 }, (_, index) => ({ Source: `/srv/data/${index}`, Destination: `/data/${index}`, Type: "bind", RW: true })),
      networks: { bridge: { IPAddress: "172.17.0.2", Gateway: "172.17.0.1" } },
    }),
    GetDockerLogs: async () => ({ text: Array.from({ length: 200 }, (_, index) => `log ${index}: ${"output ".repeat(20)}`).join("\n"), truncated: false }),
    StartDockerExec: async () => { execStarts++; return "docker-exec"; },
    ResizeSession: async (_id, rows, cols) => { resizes.push({ rows, cols }); },
    WriteSession: async () => {}, DisconnectSession: async () => {},
  } } };
  const h = React.createElement;
  function Fixture() {
    const [view, setView] = React.useState("extension");
    return h("div", { className: "session-workspace" },
      h("div", { className: "session-view pane-single has-server-extensions" },
        h("div", { className: "session-header" }, "Connected server"),
        h(SSHWorkspace, { id: "test", view, active: true, focused: false,
          extensions: ["docker"], selectedExtension: "docker", onExtensionChange: () => {}, onViewChange: setView,
          terminal: () => h("div", { className: "terminal-frame" }, "SSH terminal"),
          extension: h("div", { className: "ssh-extension-content" },
            h(DockerExtension, { sessionId: "test", active: true, focused: false, connected: true, theme: "dark" })),
        })));
  }
  const fixture = document.getElementById("fixture");
  createRoot(fixture).render(h(Fixture));
  const pause = () => new Promise((resolve) => setTimeout(resolve, 40));
  const waitFor = async (selector) => {
    for (let i = 0; i < 100 && !document.querySelector(selector); i++) await pause();
    const element = document.querySelector(selector);
    if (!element) throw new Error(`Missing ${selector}`);
    return element;
  };
  const click = async (selector) => { const element = await waitFor(selector); element.focus(); element.click(); await pause(); };
  const contained = (child, parent) => {
    const a = child.getBoundingClientRect(), b = parent.getBoundingClientRect();
    return a.top >= b.top - 1 && a.left >= b.left - 1 && a.bottom <= b.bottom + 1 && a.right <= b.right + 1;
  };
  const results = [];
  await click(".docker-select");
  await waitFor(".docker-mount");
  for (const [mode, width, height] of [["extension", 1100, 700], ["split", 1100, 700], ["extension", 480, 500], ["split", 900, 460]]) {
    fixture.style.width = `${width}px`;
    fixture.style.height = `${height}px`;
    if (!document.querySelector(`.layout-${mode}`)) {
      await click(".ssh-split-toggle");
      if (mode === "extension" && !document.querySelector(".layout-extension")) await click(".ssh-viewbar .ssh-view-button:nth-child(2)");
    }
    await pause();
    const issues = [];
    await click(".docker-detail-tabs button:nth-child(1)");
    const details = await waitFor(".docker-details-content");
    const inspector = document.querySelector(".docker-inspector");
    if (details.clientHeight < 60 || details.scrollHeight <= details.clientHeight) issues.push("Details has no usable scroll area");
    details.scrollTop = details.scrollHeight;
    if (details.scrollTop === 0 || !contained(details, inspector)) issues.push("Details cannot scroll within the inspector");
    await click(".docker-detail-tabs button:nth-child(2)");
    const logs = await waitFor(".docker-log-scroll");
    if (!logs.textContent.includes("log 199:")) issues.push("Fetched logs are missing");
    if (logs.clientHeight < 60 || !contained(logs, inspector)) issues.push("Logs are collapsed or clipped");
    logs.scrollTop = logs.scrollHeight;
    if (logs.scrollTop === 0) issues.push("Logs cannot scroll");
    await click(".docker-detail-tabs button:nth-child(3)");
    const screen = await waitFor(".docker-exec-terminal .xterm-screen");
    pushTerminalData("docker-exec", btoa("Container shell ready\r\n$ "));
    await pause();
    const canvas = document.querySelector(".docker-exec-terminal .terminal-canvas");
    const frame = document.querySelector(".docker-exec-terminal");
    if (frame.clientHeight < 60 || !contained(frame, inspector)) issues.push("Exec terminal overflows its inspector");
    if (!contained(inspector, document.querySelector(".docker-workspace"))) issues.push("Exec inspector is clipped by the workspace");
    if (!contained(canvas, frame)) issues.push("Terminal canvas overflows its frame");
    const fontSize = parseFloat(getComputedStyle(canvas.querySelector(".xterm")).fontSize);
    if (resizes.at(-1)?.rows * fontSize > canvas.clientHeight || resizes.at(-1)?.cols * fontSize * 0.5 > canvas.clientWidth) issues.push("Terminal grid was not fitted to the available space");
    if (!resizes.length || resizes.at(-1).rows < 2 || resizes.at(-1).cols < 2) issues.push("Exec terminal was not resized");
    if (!contained(document.querySelector(".docker-extension"), document.querySelector(".ssh-extension-pane"))) issues.push("Docker view overflows the session pane");
    results.push({ mode, width, height, issues, ...(issues.length ? { geometry: Object.fromEntries([
      ["frame", frame], ["canvas", canvas], ["screen", screen], ["inspector", inspector], ["panel", frame.parentElement],
    ].map(([name, element]) => [name, element.getBoundingClientRect().toJSON()])), resize: resizes.at(-1) } : {}) });
  }
  if (execStarts !== 1) results.push({ issues: ["Switching views restarted the exec session"] });
  return results;
}

test("Docker details and logs scroll, and the real exec terminal fits full and split views", { skip: !existsSync(browser) }, async () => {
  const directory = mkdtempSync(join(tmpdir(), "shell-aw-docker-layout-"));
  const source = (path) => JSON.stringify(join(root, "src", path).replaceAll("\\", "/"));
  const entry = join(directory, "entry.mjs");
  writeFileSync(entry, `import React from 'react';
    import { createRoot } from 'react-dom/client';
    import DockerExtension from ${source("components/DockerExtension.tsx")};
    import SSHWorkspace from ${source("components/SSHWorkspace.tsx")};
    import { pushTerminalData } from ${source("terminalBus.ts")};
    import ${source("index.css")}; import ${source("App.css")}; import ${source("components/ServerExtensions.css")};
    (${checkLayout.toString()})().then(results => {
      document.getElementById('result').textContent = encodeURIComponent(JSON.stringify(results));
    }).catch(error => { document.getElementById('result').textContent = encodeURIComponent(JSON.stringify([{issues:[error.message]}])); });`);
  const output = join(directory, "built");
  await build({ root, logLevel: "error", define: { "process.env.NODE_ENV": '"production"' },
    resolve: { alias: [{ find: /^react$/, replacement: require.resolve("react") }, { find: /^react-dom\/client$/, replacement: require.resolve("react-dom/client") }] },
    build: { outDir: output, emptyOutDir: true, cssCodeSplit: false,
      lib: { entry, name: "DockerLayoutTest", formats: ["iife"], fileName: () => "test.js" } },
  });
  const css = readdirSync(output).filter((name) => name.endsWith(".css")).map((name) => readFileSync(join(output, name), "utf8")).join("\n");
  const file = join(directory, "layout.html");
  writeFileSync(file, `<!doctype html><html><head><meta charset="utf-8"><style>${css}</style></head><body>
    <div id="fixture" style="width:1100px;height:700px"></div><output id="result" hidden></output><script src="built/test.js"></script></body></html>`);
  const { stdout } = await promisify(execFile)(browser, ["--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--no-sandbox",
    "--disable-crash-reporter", `--user-data-dir=${join(directory, "profile")}`, "--window-size=1400,1000", "--virtual-time-budget=15000", "--dump-dom", pathToFileURL(file).href,
  ], { windowsHide: true, timeout: 25000, maxBuffer: 2 * 1024 * 1024 });
  const encoded = stdout.match(/<output id="result" hidden="">([^<]+)<\/output>/)?.[1];
  assert.ok(encoded, "Browser should report Docker layout measurements");
  const measurements = JSON.parse(decodeURIComponent(encoded));
  assert.deepEqual(measurements.filter((measurement) => measurement.issues.length), []);
  assert.equal(measurements.length, 4);
});

