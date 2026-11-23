import assert from "node:assert/strict";
import { after, test } from "node:test";
import { fileURLToPath } from "node:url";
import { pathToFileURL } from "node:url";
import { existsSync, readFileSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer } from "vite";

const vite = await createServer({
  root: fileURLToPath(new URL("..", import.meta.url)),
  logLevel: "error",
  server: { middlewareMode: true, hmr: false, watch: null },
});
after(() => vite.close());
const { ServerInfoDashboard } = await vite.ssrLoadModule("/src/components/ServerExtensions.tsx");
const { default: SSHWorkspace } = await vite.ssrLoadModule("/src/components/SSHWorkspace.tsx");
const { historySample } = await vite.ssrLoadModule("/src/serverMetrics.ts");

const data = {
  supported: true, message: "", collectedAt: 6000, hostname: "demo-host", os: "Ubuntu", kernel: "6.8", uptimeSeconds: 12000,
  loadAverage: [0.1, 0.2, 0.3], cpu: { cores: 4, total: 2000, idle: 600, ioWait: 10 },
  memory: { total: 8 * 1024 ** 3, available: 4 * 1024 ** 3, cached: 1024 ** 3, swapTotal: 0, swapFree: 0 },
  storage: [{ mount: "/", device: "/dev/vda1", total: 100 * 1024 ** 3, used: 30 * 1024 ** 3, available: 70 * 1024 ** 3, usedPercent: 30 }],
  network: [{ name: "docker0", received: 100, sent: 200, receiveErrors: 0, sendErrors: 0 }, { name: "eth0", received: 300000, sent: 20000, receiveErrors: 0, sendErrors: 1 }],
  primaryInterface: "eth0",
  disks: [{ name: "vda", readBytes: 512000, writeBytes: 1024000, readOperations: 100, writeOperations: 50, busyMilliseconds: 1000 }],
};
const rates = {
  cpu: 25, ioWait: 2, network: [{ name: "docker0", received: 0, sent: 0 }, { name: "eth0", received: 12000, sent: 3000 }],
  disks: [{ name: "vda", read: 3000, write: 1000, readOps: 3, writeOps: 1, busy: 20 }],
};
const monitor = {
  data, rates, history: [historySample({ ...data, collectedAt: 3000 }, rates), historySample(data, rates)],
  error: "", pending: false, refresh: () => {},
};
const dashboard = (value = monitor, connected = true) => renderToStaticMarkup(createElement(ServerInfoDashboard, { monitor: value, connected }));

test("one dashboard renders all metric cards and real charts together", () => {
  const html = dashboard();
  for (const title of ["CPU", "Memory", "System", "Network", "Disk I/O", "Storage"]) assert.match(html, new RegExp(`>${title}</h3>`));
  assert.equal((html.match(/class="server-dashboard-card /g) ?? []).length, 6);
  assert.equal((html.match(/role="img"/g) ?? []).length, 4);
  assert.doesNotMatch(html, /role="tablist"|role="tab"/);
  assert.match(html, /70\.0 GB free/);
  assert.match(html, /100 GB/);
  assert.match(html, /demo-host/);
  assert.doesNotMatch(html, /NaN|undefined|Infinity/);
});

test("network chart starts with the default route and disk chart identifies its device", () => {
  const html = dashboard();
  assert.match(html, /aria-label="eth0 network throughput, recent live samples"/);
  assert.match(html, /value="eth0" selected=""/);
  assert.match(html, /aria-label="vda disk throughput, recent live samples"/);
  assert.match(html, /aria-label="Disk I\/O chart device"/);
});

test("missing counters, loading, disconnect, unsupported hosts and exec errors render usable states", () => {
  assert.match(dashboard({ ...monitor, data: null, rates: null, history: [], pending: true }), /Reading server information/);
  assert.match(dashboard(monitor, false), /Reconnect to resume/);
  assert.match(dashboard({ ...monitor, data: { ...data, supported: false, message: "Linux only" } }), /Linux only/);
  const error = dashboard({ ...monitor, error: "Exec denied" });
  assert.match(error, /role="alert"/);
  assert.match(error, /Try again/);
  assert.doesNotMatch(error, /server-dashboard-grid/);
  const missing = dashboard({ ...monitor, data: { ...data, cpu: null, memory: null, network: [], disks: [], storage: [] }, rates: null, history: [] });
  for (const label of ["Memory information", "Network counters", "Disk I/O counters", "Filesystem capacity"]) assert.match(missing, new RegExp(label));
  assert.doesNotMatch(missing, /NaN|undefined|Infinity/);
});

for (const view of ["terminal", "extension", "split"]) {
  test(`${view} layout keeps a single terminal attached and exposes the intended panes`, () => {
    const states = [];
    const html = renderToStaticMarkup(createElement(SSHWorkspace, {
      id: "session-1", view, active: true, focused: true,
      onViewChange: () => {}, extension: createElement("div", { "data-dashboard": true }),
      terminal: (state) => { states.push(state); return createElement("div", { "data-terminal": true }); },
    }));
    assert.equal((html.match(/data-terminal="true"/g) ?? []).length, 1);
    assert.equal(states[0].active, view !== "extension");
    assert.equal(states[0].focused, view !== "extension");
    assert.equal(/class="ssh-terminal-pane" hidden/.test(html), view === "extension");
    assert.equal(/class="ssh-extension-pane" hidden/.test(html), view === "terminal");
    assert.equal(html.includes('role="separator"'), view === "split");
    if (view === "split") assert.match(html, /aria-label="Resize terminal and Server load"/);
  });
}

test("removing the extension falls back to the existing terminal and hidden connections cannot focus it", () => {
  let state;
  const html = renderToStaticMarkup(createElement(SSHWorkspace, {
    id: "session-1", view: "split", active: false, focused: false, onViewChange: () => {}, extension: null,
    terminal: (value) => { state = value; return createElement("div", { "data-terminal": true }); },
  }));
  assert.equal(state.active, false);
  assert.equal(state.focused, false);
  assert.doesNotMatch(html, /Server load|Split view|role="separator"/);
  assert.match(html, /layout-terminal/);
});

const browserPath = process.env.BROWSER_BIN ?? (process.platform === "win32"
  ? "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" : "");

test("browser keeps charts inside their cards when resizing and showing hidden panes", { skip: !existsSync(browserPath) }, async () => {
  const directory = mkdtempSync(join(tmpdir(), "shell-aw-dashboard-layout-"));
  const file = join(directory, "layout.html");
  const css = ["index.css", "App.css", "components/ServerExtensions.css"].map((name) =>
    readFileSync(new URL(`../src/${name}`, import.meta.url), "utf8")).join("\n");
  const markup = renderToStaticMarkup(createElement("div", { className: "session-workspace" },
    createElement("div", { className: "session-view pane-single has-server-extensions" },
      createElement("div", { className: "session-header" }, "Connected server"),
      createElement(SSHWorkspace, {
        id: "browser-session", view: "extension", active: true, focused: true, onViewChange: () => {},
        extension: createElement(ServerInfoDashboard, { monitor, connected: true }),
        terminal: () => createElement("div", { className: "terminal-frame" }, "Existing terminal"),
      }))));
  const measure = () => {
    const results = [];
    const fixture = document.getElementById("fixture");
    const panes = fixture.querySelector(".ssh-view-panes");
    const terminal = fixture.querySelector(".ssh-terminal-pane");
    const extension = fixture.querySelector(".ssh-extension-pane");
    const rect = (element) => element.getBoundingClientRect();
    for (const theme of ["dark", "light"]) {
      document.documentElement.dataset.theme = theme;
      for (const [mode, width, ratio] of [["extension", 1400, 50], ["extension", 720, 50], ["extension", 280, 50], ["split", 1200, 50], ["split", 1000, 80], ["split", 1000, 20]]) {
        // Start hidden to exercise the same CSS transition as switching views.
        extension.hidden = true;
        void extension.offsetWidth;
        fixture.style.width = `${width}px`;
        panes.className = `ssh-view-panes layout-${mode}`;
        panes.style.setProperty("--ssh-terminal-ratio", `${ratio}%`);
        terminal.hidden = mode === "extension";
        extension.hidden = false;
        const issues = [];
        const body = extension.querySelector(".server-dashboard-body");
        if (body.scrollWidth > body.clientWidth + 1) issues.push("dashboard overflows horizontally");
        const charts = Array.from(extension.querySelectorAll(".server-chart"));
        for (const chart of charts) {
          const plot = rect(chart.querySelector(".server-chart-plot"));
          const canvas = rect(chart.querySelector(".server-chart-canvas"));
          const svg = rect(chart.querySelector("svg"));
          const card = rect(chart.closest(".server-dashboard-card"));
          const name = chart.querySelector("svg").getAttribute("aria-label");
          if (plot.height < 60 || canvas.height < 60 || canvas.width < 30) issues.push(`${name}: graph is hidden or collapsed`);
          if (canvas.top < plot.top - 1 || canvas.bottom > plot.bottom + 1) issues.push(`${name}: canvas height ${canvas.height} exceeds plot height ${plot.height}`);
          if (svg.left < canvas.left - 1 || svg.right > canvas.right + 1 || svg.top < canvas.top - 1 || svg.bottom > canvas.bottom + 1) issues.push(`${name}: SVG overflows canvas`);
          if (svg.right > card.right + 1 || svg.bottom > card.bottom + 1) issues.push(`${name}: graph clipped by card`);
          for (const reading of chart.querySelectorAll(".server-chart-reading")) {
            if (reading.scrollWidth > reading.clientWidth + 1) issues.push(`${name}: reading overflows legend`);
          }
        }
        results.push({ theme, mode, width, ratio, charts: charts.length, issues });
      }
    }
    document.getElementById("layout-results").textContent = encodeURIComponent(JSON.stringify(results));
  };
  writeFileSync(file, `<!doctype html><html><head><meta charset="utf-8"><style>${css}</style></head><body>
    <div id="fixture" style="width:1400px;height:700px">${markup}</div>
    <pre id="layout-results"></pre><script>(${measure.toString()})()</script></body></html>`);
  const { stdout } = await promisify(execFile)(browserPath, [
    "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--no-sandbox",
    "--disable-crash-reporter", `--user-data-dir=${join(directory, "profile")}`, "--window-size=1600,1000",
    "--dump-dom", pathToFileURL(file).href,
  ], { windowsHide: true, timeout: 20000, maxBuffer: 1024 * 1024 });
  const encoded = stdout.match(/<pre id="layout-results">([^<]+)<\/pre>/)?.[1];
  assert.ok(encoded, "browser should report chart measurements");
  const measurements = JSON.parse(decodeURIComponent(encoded));
  assert.equal(measurements.length, 12);
  for (const measurement of measurements) {
    assert.equal(measurement.charts, 4);
    assert.deepEqual(measurement.issues, [], JSON.stringify(measurement));
  }
});
