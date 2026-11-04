import { execFileSync, spawnSync } from "node:child_process";
import { after, test } from "node:test";
import assert from "node:assert/strict";
import {
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";

const directory = mkdtempSync(join(tmpdir(), "shell-anywhere-release-"));
mkdirSync(join(directory, "scripts"));
const script = join(directory, "scripts", "release-version.mjs");
const config = join(directory, "wails.json");
copyFileSync(new URL("./release-version.mjs", import.meta.url), script);

after(() => {
  assert.equal(dirname(resolve(directory)), resolve(tmpdir()));
  assert.ok(basename(directory).startsWith("shell-anywhere-release-"));
  rmSync(directory, { recursive: true, force: true });
});

test("release and prerelease tags keep a numeric app version and the full build tag", () => {
  for (const tag of ["v1.2.3", "1.2.3", "v1.2.3-beta.1+build.42"]) {
    writeFileSync(config, JSON.stringify({ name: "Shell Anywhere", info: {} }));
    execFileSync(process.execPath, [script, tag]);
    const result = JSON.parse(readFileSync(config, "utf8"));
    assert.equal(result.name, "Shell Anywhere");
    assert.equal(result.info.productVersion, "1.2.3");
    assert.equal(result.info.comments, `Desktop SSH client - ${tag}`);
  }
});

test("invalid or unpackageable tags fail before changing configuration", () => {
  const original = JSON.stringify({ info: { productVersion: "1.0.0" } });
  for (const tag of [
    "",
    "v1.2",
    "v01.2.3",
    "v65536.1.0",
    "v1.2.3\n",
    "v1.2.3/other",
    "v1.2.3;echo bad",
  ]) {
    writeFileSync(config, original);
    assert.notEqual(spawnSync(process.execPath, [script, tag]).status, 0);
    assert.equal(readFileSync(config, "utf8"), original);
  }
});
