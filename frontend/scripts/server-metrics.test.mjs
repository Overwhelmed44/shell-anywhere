import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";
import ts from "typescript";

const source = readFileSync(new URL("../src/serverMetrics.ts", import.meta.url), "utf8");
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText;
const context = { exports: {} };
vm.runInNewContext(compiled, context);
const { serverRates, historySample, primaryNetwork, busiestDisk, serverExtensionIds, validServerExtensions, formatBytes, formatRate, percent } = context.exports;
const chartSource = readFileSync(new URL("../src/serverMetricChart.ts", import.meta.url), "utf8");
const chartContext = { exports: {} };
vm.runInNewContext(ts.transpileModule(chartSource, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText, chartContext);
const { metricChartPath } = chartContext.exports;

function sample() {
  return {
    supported: true, message: "", collectedAt: 1000, hostname: "test", os: "Linux", kernel: "6.8", uptimeSeconds: 100,
    loadAverage: [0.1, 0.2, 0.3], cpu: { cores: 4, total: 1000, idle: 600, ioWait: 10 },
    memory: { total: 1000, available: 400, cached: 200, swapTotal: 200, swapFree: 100 }, storage: [],
    network: [{ name: "eth0", received: 1000, sent: 3000, receiveErrors: 0, sendErrors: 0 }], primaryInterface: "eth0",
    disks: [{ name: "vda", readBytes: 512000, writeBytes: 1024000, readOperations: 100, writeOperations: 50, busyMilliseconds: 1000 }],
  };
}

test("a first snapshot never invents CPU, traffic or I/O rates", () => {
  const data = sample();
  const rates = serverRates(data, null);
  assert.equal(rates.cpu, null);
  assert.equal(rates.network[0].received, null);
  assert.equal(rates.disks[0].read, null);
  assert.equal(historySample(data, rates).memory, 60);
});

test("calculates rates from elapsed server time and cumulative counters", () => {
  const previous = sample();
  const current = sample();
  current.uptimeSeconds = 103;
  current.cpu = { cores: 4, total: 1200, idle: 750, ioWait: 20 };
  current.network[0].received = 4000;
  current.network[0].sent = 6000;
  current.disks[0] = { name: "vda", readBytes: 518144, writeBytes: 1036288, readOperations: 106, writeOperations: 53, busyMilliseconds: 1900 };
  const rates = serverRates(current, previous);
  assert.equal(rates.cpu, 25);
  assert.equal(rates.ioWait, 5);
  assert.equal(rates.network[0].received, 1000);
  assert.equal(rates.disks[0].read, 2048);
  assert.equal(rates.disks[0].write, 4096);
  assert.equal(rates.disks[0].readOps, 2);
  assert.equal(rates.disks[0].busy, 30);
});

test("a reboot, stale sample or zero elapsed time requires a new baseline", () => {
  for (const uptime of [10, 100, 150]) {
    const previous = sample();
    const current = sample();
    current.uptimeSeconds = uptime;
    current.cpu.total += 100;
    current.network[0].received += 1000;
    const rates = serverRates(current, previous);
    assert.equal(rates.cpu, null);
    assert.equal(rates.network[0].received, null);
  }
});

test("device resets, new interfaces and CPU hotplug do not produce spikes", () => {
  const previous = sample();
  const current = sample();
  current.uptimeSeconds = 103;
  current.cpu.cores = 8;
  current.network[0].received = 0;
  current.network.push({ name: "eth1", received: 500000, sent: 100000 });
  current.disks[0].busyMilliseconds = 0;
  const rates = serverRates(current, previous);
  assert.equal(rates.cpu, null);
  assert.equal(rates.network[0].received, null);
  assert.equal(rates.network[1].received, null);
  assert.equal(rates.disks[0].busy, null);
});

test("server info is one extension and old metric pins migrate together", () => {
  assert.equal(JSON.stringify(serverExtensionIds), '["server-info"]');
  assert.equal(JSON.stringify(validServerExtensions([])), "[]");
  assert.equal(JSON.stringify(validServerExtensions(["memory", "fake", "memory", "overview"])), '["server-info"]');
  assert.equal(JSON.stringify(validServerExtensions(["storage", "network", "io"])), '["server-info"]');
  assert.equal(JSON.stringify(validServerExtensions(["server-info", "server-info"])), '["server-info"]');
  assert.equal(JSON.stringify(validServerExtensions(["fake"])), '[]');
  assert.equal(JSON.stringify(validServerExtensions(null)), '["server-info"]');
});

test("uses the default route and a single busiest disk rather than double-counting devices", () => {
  const data = sample();
  const rates = { cpu: null, ioWait: null, network: [{ name: "docker0", received: 5, sent: 2 }, { name: "eth0", received: 20, sent: 10 }], disks: [{ name: "vda", read: 20, write: 50 }, { name: "dm-0", read: 10, write: 30 }] };
  assert.equal(primaryNetwork(data, rates).name, "eth0");
  assert.equal(busiestDisk(rates).name, "vda");
  assert.equal(rates.disks.length, 2);
});

test("formats unknown, zero and large readings without NaN or infinity", () => {
  assert.equal(formatBytes(0), "0 B");
  assert.equal(formatBytes(1024), "1.00 KB");
  assert.equal(formatBytes(Number.NaN), "—");
  assert.equal(formatRate(null), "—");
  assert.equal(percent(110, 100), 100);
  assert.equal(percent(1, 0), 0);
});

test("dashboard history preserves each interface and disk instead of mixing the busiest devices", () => {
  const previous = sample();
  const current = sample();
  current.uptimeSeconds = 103;
  current.network[0].received += 3000;
  current.network.push({ name: "docker0", received: 999999, sent: 999999, receiveErrors: 0, sendErrors: 0 });
  current.disks[0].readBytes += 6144;
  current.disks.push({ ...current.disks[0], name: "dm-0", readBytes: 999999 });
  const history = historySample(current, serverRates(current, previous));
  assert.equal(history.network.find((item) => item.name === "eth0").received, 1000);
  assert.equal(history.network.find((item) => item.name === "docker0").received, null);
  assert.equal(history.disks.find((item) => item.name === "vda").read, 2048);
  assert.equal(history.disks.find((item) => item.name === "dm-0").read, null);
});

test("charts position samples by elapsed time and leave unknown counters as gaps", () => {
  const path = metricChartPath([
    { at: 1000, value: 0 }, { at: 2000, value: 50 },
    { at: 3000, value: null }, { at: 5000, value: 100 }, { at: 6000, value: 25 },
  ], 100);
  assert.equal(path.trim(), "M0.00 100.00 L96.00 50.00  M384.00 0.00 L480.00 75.00");
});

test("charts never emit invalid SVG coordinates or draw without an elapsed baseline", () => {
  assert.equal(metricChartPath([{ at: 1, value: 2 }], 100), "");
  assert.equal(metricChartPath([{ at: 1, value: 2 }, { at: 1, value: 3 }], 100), "");
  assert.equal(metricChartPath([{ at: 1, value: 2 }, { at: 2, value: 3 }], 0), "");
  const path = metricChartPath([
    { at: 1, value: -10 }, { at: 2, value: Number.NaN },
    { at: 3, value: 120 }, { at: 4, value: Number.POSITIVE_INFINITY },
  ], 100);
  assert.equal(path.includes("NaN"), false);
  assert.equal(path.includes("Infinity"), false);
  assert.equal(path.includes("L"), false);
  assert.match(path, /M0\.00 100\.00/);
  assert.match(path, /M320\.00 0\.00/);
});
