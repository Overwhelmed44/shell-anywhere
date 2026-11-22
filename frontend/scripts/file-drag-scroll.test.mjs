import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";
import ts from "typescript";

const source = readFileSync(new URL("../src/components/fileDragScroll.ts", import.meta.url), "utf8");
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText;

function fixture() {
  let nextId = 0;
  let time = 0;
  let top = 300;
  const frames = new Map();
  const movement = [];
  const element = {
    isConnected: true,
    clientHeight: 300,
    clientWidth: 300,
    scrollHeight: 1200,
    scrollLeft: 0,
    getBoundingClientRect: () => ({ left: 10, right: 310, top: 100, bottom: 400, height: 300 }),
    get scrollTop() { return top; },
    set scrollTop(value) { top = Math.max(0, Math.min(900, value)); },
  };
  const context = {
    exports: {},
    window: {
      requestAnimationFrame(callback) { frames.set(++nextId, callback); return nextId; },
      cancelAnimationFrame(id) { frames.delete(id); },
    },
  };
  vm.runInNewContext(compiled, context);
  const scrolling = context.exports.createFileDragScroll(element, (x, y) => movement.push({ x, y, top }));
  const advance = () => {
    time += 1000 / 60;
    const callbacks = [...frames.values()];
    frames.clear();
    callbacks.forEach((callback) => callback(time));
  };
  return { scrolling, element, frames, movement, advance };
}

test("scrolls continuously at the bottom edge and updates the target after moving", () => {
  const { scrolling, element, movement, advance } = fixture();
  scrolling.update(150, 395);
  advance();
  const first = element.scrollTop;
  assert.ok(first > 300);
  advance();
  assert.ok(element.scrollTop > first);
  assert.equal(movement.length, 2);
  assert.deepEqual(movement[1], { x: 150, y: 395, top: element.scrollTop });
});

test("scrolls up and accelerates closer to the edge", () => {
  const near = fixture();
  const far = fixture();
  near.scrolling.update(150, 102);
  far.scrolling.update(150, 165);
  near.advance();
  far.advance();
  assert.ok(near.element.scrollTop < far.element.scrollTop);
  assert.ok(far.element.scrollTop < 300);
});

test("does not scroll in the middle or outside the list", () => {
  const { scrolling, frames } = fixture();
  for (const [x, y] of [[150, 250], [0, 395], [150, 99], [150, 400]]) {
    scrolling.update(x, y);
    assert.equal(frames.size, 0);
  }
});

test("moving away from an edge or ending the drag cancels pending scrolling", () => {
  const { scrolling, element, frames, advance } = fixture();
  scrolling.update(150, 395);
  scrolling.update(150, 250);
  assert.equal(frames.size, 0);
  scrolling.update(150, 395);
  scrolling.stop();
  advance();
  assert.equal(element.scrollTop, 300);
  assert.equal(frames.size, 0);
});

test("stops at both filesystem boundaries", () => {
  const { scrolling, element, frames, advance } = fixture();
  element.scrollTop = 899;
  scrolling.update(150, 399);
  advance();
  advance();
  assert.equal(element.scrollTop, 900);
  assert.equal(frames.size, 0);
  element.scrollTop = 1;
  scrolling.update(150, 101);
  advance();
  advance();
  assert.equal(element.scrollTop, 0);
  assert.equal(frames.size, 0);
});

test("stops if the pane is removed while dragging", () => {
  const { scrolling, element, frames, advance } = fixture();
  scrolling.update(150, 395);
  element.isConnected = false;
  advance();
  assert.equal(element.scrollTop, 300);
  assert.equal(frames.size, 0);
});

test("wheel scrolling works in the middle and retargets the folder immediately", () => {
  const { scrolling, element, movement, frames } = fixture();
  scrolling.update(150, 250);
  assert.equal(frames.size, 0);
  assert.equal(scrolling.wheel(0, 120, 0), true);
  assert.equal(element.scrollTop, 420);
  assert.deepEqual(movement.at(-1), { x: 150, y: 250, top: 420 });
});

test("normalizes line, page and horizontal wheel deltas", () => {
  const { scrolling, element } = fixture();
  scrolling.update(150, 250);
  scrolling.wheel(1, 2, 1);
  assert.equal(element.scrollTop, 332);
  assert.equal(element.scrollLeft, 16);
  scrolling.wheel(0, 1, 2);
  assert.equal(element.scrollTop, 632);
});

test("manual wheel scrolling pauses edge scrolling instead of fighting it", () => {
  const { scrolling, element, frames, advance } = fixture();
  scrolling.update(150, 395);
  assert.equal(frames.size, 1);
  scrolling.wheel(0, -100, 0);
  assert.equal(element.scrollTop, 200);
  assert.equal(frames.size, 0);
  scrolling.update(150, 395);
  advance();
  assert.equal(element.scrollTop, 200);
});

test("wheel handling ends with the drag and does not hijack ordinary scrolling", () => {
  const { scrolling, element } = fixture();
  assert.equal(scrolling.wheel(0, 100, 0), false);
  scrolling.update(150, 250);
  scrolling.stop();
  assert.equal(scrolling.wheel(0, 100, 0), false);
  assert.equal(element.scrollTop, 300);
});
