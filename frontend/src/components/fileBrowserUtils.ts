import type { FileEntry } from "../api";

export function sizeText(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unit]}`;
}

export function breadcrumbs(path: string): { name: string; path: string }[] {
  const separator = path.includes("\\") ? "\\" : "/";
  const normalized = path.replaceAll("\\", "/");
  const root = normalized.match(/^([a-z]:\/|\/\/[^/]+\/[^/]+\/?|\/)/i)?.[0] ?? "";
  const parts = normalized.slice(root.length).split("/").filter(Boolean);
  const result = root
    ? [{ name: root === "/" ? "/" : root.replace(/\/$/, ""), path: root.replaceAll("/", separator) }]
    : [];
  parts.forEach((name, index) => {
    result.push({ name, path: `${root}${root && !root.endsWith("/") ? "/" : ""}${parts.slice(0, index + 1).join("/")}`.replaceAll("/", separator) });
  });
  return result;
}

export type FileKind = "folder" | "link" | "code" | "image" | "archive" | "text" | "media" | "file";

export function fileKind(entry: FileEntry): FileKind {
  if (entry.symlink) return "link";
  if (entry.directory) return "folder";
  const name = entry.name.toLowerCase();
  const extension = name.split(".").pop() ?? "";
  if (/^(png|jpe?g|gif|webp|svg|ico|avif|bmp)$/.test(extension)) return "image";
  if (/^(zip|gz|tar|7z|rar|bz2|xz|zst)$/.test(extension)) return "archive";
  if (/^(mp4|mov|webm|mp3|wav|ogg|flac)$/.test(extension)) return "media";
  if (/^(json|ya?ml|toml|xml|js|jsx|ts|tsx|py|go|rs|sh|css|html|conf|ini|env|sql)$/.test(extension) || /^(dockerfile|makefile|\.gitignore|\.env)$/.test(name)) return "code";
  if (/^(txt|md|log|csv|pdf|docx?)$/.test(extension)) return "text";
  return "file";
}

export type FileSort = { key: "name" | "size" | "modified"; descending: boolean };

export function visibleFiles(entries: FileEntry[], query: string, showHidden: boolean, sort: FileSort): FileEntry[] {
  const search = query.trim().toLowerCase();
  return entries.filter((entry) => (showHidden || !entry.name.startsWith(".")) && entry.name.toLowerCase().includes(search)).sort((a, b) => {
    if (a.directory !== b.directory) return a.directory ? -1 : 1;
    const compared = sort.key === "name"
      ? a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" })
      : a[sort.key] - b[sort.key];
    return (compared || a.name.localeCompare(b.name)) * (sort.descending ? -1 : 1);
  });
}
