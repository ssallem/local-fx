import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Resvg } from "@resvg/resvg-js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const iconsDir = path.join(__dirname, "..", "public", "icons");

const svgPath = path.join(iconsDir, "icon.svg");
const svg = fs.readFileSync(svgPath);

for (const size of [16, 48, 128]) {
  const resvg = new Resvg(svg, { fitTo: { mode: "width", value: size } });
  const png = resvg.render().asPng();
  const out = path.join(iconsDir, `icon-${size}.png`);
  fs.writeFileSync(out, png);
  console.log(`✓ ${out} (${png.length} bytes)`);
}
