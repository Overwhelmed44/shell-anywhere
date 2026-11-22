import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";
import ts from "typescript";

const source = readFileSync(new URL("../src/components/filePointerDrag.ts", import.meta.url), "utf8");
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText;

class InputEvent extends Event {
  constructor(type, options = {}) {
    super(type, { cancelable: true });
    for (const [key, value] of Object.entries(options)) {
      if (key !== "bubbles" && key !== "cancelable") this[key] = value;
    }
  }
}

function fixture() {
  const window = new EventTarget();
  window.innerWidth = 1280;
  window.innerHeight = 720;
  const timers = [];
  window.setTimeout = (callback) => timers.push(callback);
  const document = new EventTarget();
  document.hidden = false;
  const children = [];
  document.body = { append: (element) => children.push(element) };
  document.createElement = () => ({
    style: {},
    classList: { toggle() {} },
    offsetWidth: 240,
    offsetHeight: 60,
    append() {},
    remove() { const index = children.indexOf(this); if (index >= 0) children.splice(index, 1); },
  });
  const root = {};
  const pane = {};
  let captured = false;
  const source = new EventTarget();
  source.closest = () => root;
  source.setPointerCapture = () => { captured = true; };
  source.hasPointerCapture = () => captured;
  source.releasePointerCapture = () => { captured = false; };
  const drops = [];
  function folder(name) {
    const target = new EventTarget();
    target.closest = (selector) => selector === ".file-browser" ? root : pane;
    target.addEventListener("dragover", (event) => event.preventDefault());
    target.addEventListener("drop", (event) => drops.push({ name, data: event.dataTransfer.getData("files") }));
    return target;
  }
  let hit = folder("first");
  document.elementFromPoint = () => hit;
  let starts = 0;
  let ends = 0;
  class Transfer {
    values = new Map();
    setData(type, data) { this.values.set(type, data); }
    getData(type) { return this.values.get(type); }
  }
  const context = { exports: {}, window, document, DataTransfer: Transfer, DragEvent: InputEvent };
  vm.runInNewContext(compiled, context);
  const stop = context.exports.startFilePointerDrag(source,
    new InputEvent("pointerdown", { pointerId: 1, clientX: 10, clientY: 10 }), {
      type: "files", data: '["/srv/test.txt"]', label: "test.txt", count: 1,
      onStart: () => starts++, onEnd: () => ends++,
    });
  const send = (type, options = {}) => {
    const event = new InputEvent(type, { pointerId: 1, clientX: 100, clientY: 100, ...options });
    window.dispatchEvent(event);
    return event;
  };
  return {
    send, stop, children, drops, folder,
    setTarget(target) { hit = target; },
    get starts() { return starts; }, get ends() { return ends; }, get captured() { return captured; },
    flush: () => timers.splice(0).forEach((callback) => callback()),
  };
}

test("clicking or small pointer movements do not start a transfer", () => {
  const f = fixture();
  f.send("pointermove", { clientX: 12, clientY: 12 });
  f.send("pointerup");
  assert.equal(f.starts, 0);
  assert.equal(f.drops.length, 0);
  assert.equal(f.children.length, 0);
  assert.equal(f.captured, false);
});

test("pointer dragging preserves wheel input and drops the selected file payload", () => {
  const f = fixture();
  f.send("pointermove");
  assert.equal(f.starts, 1);
  assert.equal(f.children.length, 1);
  assert.equal(f.send("wheel", { deltaY: 120 }).defaultPrevented, false);
  f.send("pointerup");
  assert.deepEqual(f.drops, [{ name: "first", data: '["/srv/test.txt"]' }]);
  assert.equal(f.children.length, 0);
  assert.equal(f.ends, 1);
  assert.equal(f.send("click").defaultPrevented, true);
  f.flush();
  assert.equal(f.send("click").defaultPrevented, false);
});

test("drop uses the folder currently under the pointer after scrolling", () => {
  const f = fixture();
  f.send("pointermove");
  f.setTarget(f.folder("revealed by scrolling"));
  f.send("pointerup");
  assert.equal(f.drops[0].name, "revealed by scrolling");
});

test("Escape, pointer cancellation and window blur cancel without a drop", () => {
  for (const [type, options] of [["keydown", { key: "Escape" }], ["pointercancel", {}], ["blur", {}]]) {
    const f = fixture();
    f.send("pointermove");
    f.send(type, options);
    f.send("pointerup");
    assert.equal(f.drops.length, 0);
    assert.equal(f.ends, 1);
    assert.equal(f.children.length, 0);
    assert.equal(f.captured, false);
  }
});

test("dropping outside the file browser cancels and releases pointer capture", () => {
  const f = fixture();
  f.send("pointermove");
  f.setTarget(null);
  f.send("pointerup");
  assert.equal(f.drops.length, 0);
  assert.equal(f.captured, false);
});

test("cleanup is idempotent and ignores other pointers", () => {
  const f = fixture();
  f.send("pointermove", { pointerId: 2 });
  assert.equal(f.starts, 0);
  f.stop();
  f.stop();
  f.send("pointermove");
  assert.equal(f.starts, 0);
  assert.equal(f.ends, 1);
});
