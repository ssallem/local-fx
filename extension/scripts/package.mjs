import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import { fileURLToPath } from "node:url";
import archiver from "archiver";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, "..");

// 1. vite build
console.log("→ vite build");
const buildResult = spawnSync("npx", ["vite", "build"], {
  cwd: root,
  stdio: "inherit",
  shell: true,
});
if (buildResult.status !== 0) process.exit(buildResult.status);

// 2. manifest strip key
const manifestPath = path.join(root, "dist", "manifest.json");
const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
const version = manifest.version;
if (manifest.key) {
  console.log(
    `→ stripping 'key' field from dist/manifest.json (was ${manifest.key.slice(
      0,
      16
    )}...)`
  );
  delete manifest.key;
  fs.writeFileSync(manifestPath, JSON.stringify(manifest, null, 2));
}

// 3. zip
const outDir = path.join(root, "dist-prod");
fs.mkdirSync(outDir, { recursive: true });
const zipPath = path.join(outDir, `localfx-v${version}.zip`);
const output = fs.createWriteStream(zipPath);
const archive = archiver("zip", { zlib: { level: 9 } });
archive.pipe(output);
archive.directory(path.join(root, "dist"), false);

await new Promise((resolve, reject) => {
  output.on("close", resolve);
  archive.on("error", reject);
  archive.finalize();
});

console.log(`→ ${zipPath} (${output.bytesWritten} bytes)`);

// 4. checksum
const buf = fs.readFileSync(zipPath);
const sha256 = crypto.createHash("sha256").update(buf).digest("hex");
fs.writeFileSync(
  path.join(outDir, "SHASUM256.txt"),
  `${sha256}  localfx-v${version}.zip\n`
);
console.log(`→ SHA-256: ${sha256}`);
