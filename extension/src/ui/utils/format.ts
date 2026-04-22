// Small pure helpers used by Toolbar/Breadcrumb/FileList/StatusBar.
// Kept deliberately dependency-free so a Vitest pass later is trivial.

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"] as const;

/**
 * Human-readable file size. Uses 1024-based units (matches Windows Explorer).
 * null → "—"  (directories serialise sizeBytes as null per PROTOCOL.md §7.3).
 */
export function formatBytes(n: number | null | undefined): string {
  if (n === null || n === undefined) return "—"; // em dash
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n === 0) return "0 B";

  let i = 0;
  let v = n;
  while (v >= 1024 && i < UNITS.length - 1) {
    v /= 1024;
    i++;
  }
  // 1 decimal for KB+, 0 for bytes.
  const formatted = i === 0 ? v.toFixed(0) : v.toFixed(v >= 100 ? 0 : 1);
  return `${formatted} ${UNITS[i]}`;
}

/**
 * ms (unix millis) → locale string. 0/invalid → "—".
 */
export function formatTime(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return "—";
  if (!Number.isFinite(ms) || ms <= 0) return "—";
  try {
    return new Date(ms).toLocaleString();
  } catch {
    return "—";
  }
}

/**
 * Break a path into breadcrumb segments.
 *
 *   Windows: "C:\\a\\b"         → [{ label: "C:\\", path: "C:\\" },
 *                                   { label: "a",    path: "C:\\a" },
 *                                   { label: "b",    path: "C:\\a\\b" }]
 *   macOS:   "/Users/a/b"       → [{ label: "/",    path: "/" },
 *                                   { label: "Users", path: "/Users" }, ...]
 *
 * The first element is always the root with a trailing separator so clicking
 * it navigates back to the drive/volume root rather than an empty string.
 */
export interface PathSegment {
  label: string;
  path: string;
}

export function splitPath(path: string): PathSegment[] {
  if (!path) return [];

  // Windows drive root like "C:" / "C:\" / "C:/"
  const winMatch = /^([a-zA-Z]):[\\/]?/.exec(path);
  if (winMatch) {
    const drive = `${winMatch[1]}:\\`;
    const rest = path.slice(winMatch[0].length);
    const parts = rest.split(/[\\/]+/).filter(Boolean);
    const segs: PathSegment[] = [{ label: drive, path: drive }];
    let acc = drive;
    for (const p of parts) {
      acc = acc.endsWith("\\") ? `${acc}${p}` : `${acc}\\${p}`;
      segs.push({ label: p, path: acc });
    }
    return segs;
  }

  // POSIX
  if (path.startsWith("/")) {
    const parts = path.split("/").filter(Boolean);
    const segs: PathSegment[] = [{ label: "/", path: "/" }];
    let acc = "";
    for (const p of parts) {
      acc = `${acc}/${p}`;
      segs.push({ label: p, path: acc });
    }
    return segs;
  }

  // Fallback — unknown style, return as single segment.
  return [{ label: path, path }];
}

/**
 * Pick the platform separator for a path. Windows paths begin with a drive
 * letter; POSIX paths begin with `/`. Unknown styles default to `/`.
 */
function sepFor(path: string): "\\" | "/" {
  if (/^[a-zA-Z]:[\\/]?/.test(path)) return "\\";
  return "/";
}

/**
 * Join a parent path with a child segment, choosing the separator from the
 * parent. Tolerates trailing separators on the parent.
 *   "C:\\a",  "b" → "C:\\a\\b"
 *   "C:\\",   "b" → "C:\\b"
 *   "/home",  "b" → "/home/b"
 *   "/",      "b" → "/b"
 */
export function joinPath(parent: string, child: string): string {
  if (!parent) return child;
  const sep = sepFor(parent);
  const last = parent[parent.length - 1];
  if (last === "\\" || last === "/") return `${parent}${child}`;
  return `${parent}${sep}${child}`;
}

/**
 * Last path segment. Intended for display — the Entry.name field should be
 * preferred when available.
 *   "C:\\a\\b.txt" → "b.txt"
 *   "/a/b.txt"      → "b.txt"
 *   "C:\\"          → "C:\\"
 *   "/"             → "/"
 */
export function basename(path: string): string {
  if (!path) return "";
  if (/^[a-zA-Z]:[\\/]?$/.test(path)) return path;
  if (path === "/") return "/";
  const idx = Math.max(path.lastIndexOf("\\"), path.lastIndexOf("/"));
  if (idx < 0) return path;
  return path.slice(idx + 1) || path;
}

/**
 * Parent path. Returns null when already at a root.
 *   "C:\\a\\b" → "C:\\a"
 *   "C:\\a"    → "C:\\"
 *   "C:\\"     → null   (caller should flip to drive-list screen)
 *   "/a/b"     → "/a"
 *   "/a"       → "/"
 *   "/"        → null
 */
export function parentPath(path: string): string | null {
  if (!path) return null;

  // Windows drive root
  if (/^[a-zA-Z]:[\\/]?$/.test(path)) return null;

  // Windows path
  if (/^[a-zA-Z]:[\\/]/.test(path)) {
    const idx = Math.max(path.lastIndexOf("\\"), path.lastIndexOf("/"));
    if (idx < 0) return null;
    const head = path.slice(0, idx);
    // "C:" → pop back up to "C:\\"
    if (/^[a-zA-Z]:$/.test(head)) return `${head[0]}:\\`;
    return head;
  }

  // POSIX root
  if (path === "/") return null;
  if (path.startsWith("/")) {
    const idx = path.lastIndexOf("/");
    if (idx <= 0) return "/";
    return path.slice(0, idx);
  }

  return null;
}
