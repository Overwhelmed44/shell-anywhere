import { readFileSync, writeFileSync } from "node:fs";

const tag = process.argv[2] ?? "";
const version =
  /^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.exec(
    tag,
  );

if (
  !version ||
  version[0] !== tag ||
  version.slice(1, 4).some((part) => Number(part) > 65535)
) {
  throw new Error(
    "Use a release tag such as v1.2.3, with numeric components between 0 and 65535",
  );
}

const path = new URL("../wails.json", import.meta.url);
const config = JSON.parse(readFileSync(path, "utf8"));
config.info.productVersion = version.slice(1, 4).join(".");
config.info.comments = `Desktop SSH client - ${tag}`;
writeFileSync(path, JSON.stringify(config, null, 2) + "\n");
