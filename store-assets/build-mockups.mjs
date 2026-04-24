// Chrome Web Store screenshot mockup builder.
// Generates 5 x 1280x800 PNGs under ./screenshots/.
// Run: node build-mockups.mjs

import { Resvg } from "@resvg/resvg-js";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const outDir = path.join(__dirname, "screenshots");
fs.mkdirSync(outDir, { recursive: true });

// Design tokens
const COLOR = {
  bg: "#f7f8fa",
  panel: "#ffffff",
  border: "#d7dbe0",
  borderSoft: "#e5e7eb",
  text: "#1f2937",
  textDim: "#6b7280",
  textMuted: "#9ca3af",
  accent: "#1a73e8",
  accentSoft: "#e8f0fe",
  accentDark: "#1557b0",
  success: "#22c55e",
  warning: "#f59e0b",
  warningSoft: "#fff7e0",
  warningBorder: "#f0b400",
  danger: "#dc2626",
  sidebar: "#f1f3f5",
  row: "#ffffff",
  rowAlt: "#fafbfc",
  rowSel: "#e7f0ff",
  header: "#eef0f3",
  chrome: "#edeef0",
};

const FONT = "'Segoe UI', 'Malgun Gothic', system-ui, sans-serif";

// Simple document/folder glyphs (SVG paths) instead of emoji.
function folderIcon(x, y, color = "#f2c14e") {
  return `<g transform="translate(${x},${y})">
    <path d="M0 3 C0 1.3 1.3 0 3 0 L7 0 L10 3 L17 3 C18.7 3 20 4.3 20 6 L20 14 C20 15.7 18.7 17 17 17 L3 17 C1.3 17 0 15.7 0 14 Z"
      fill="${color}" stroke="#c99a2a" stroke-width="0.6"/>
  </g>`;
}
function fileIcon(x, y, color = "#6ea8fe") {
  return `<g transform="translate(${x},${y})">
    <path d="M2 0 L13 0 L18 5 L18 18 C18 19.1 17.1 20 16 20 L2 20 C0.9 20 0 19.1 0 18 L0 2 C0 0.9 0.9 0 2 0 Z"
      fill="${color}" stroke="#3e6fc7" stroke-width="0.6"/>
    <path d="M13 0 L13 5 L18 5 Z" fill="#b8d0fa" stroke="#3e6fc7" stroke-width="0.6"/>
  </g>`;
}
function driveIcon(x, y) {
  return `<g transform="translate(${x},${y})">
    <rect x="0" y="4" width="36" height="22" rx="3" fill="#e2e8f0" stroke="#94a3b8" stroke-width="1"/>
    <rect x="0" y="4" width="36" height="8" rx="3" fill="#cbd5e1"/>
    <circle cx="30" cy="20" r="2" fill="#22c55e"/>
    <rect x="5" y="16" width="16" height="2" rx="1" fill="#94a3b8"/>
    <rect x="5" y="20" width="12" height="2" rx="1" fill="#94a3b8"/>
  </g>`;
}

// Browser chrome header
function browserChrome() {
  return `
    <rect x="0" y="0" width="1280" height="48" fill="${COLOR.chrome}"/>
    <circle cx="24" cy="24" r="6" fill="#ff5f56"/>
    <circle cx="44" cy="24" r="6" fill="#ffbd2e"/>
    <circle cx="64" cy="24" r="6" fill="#27c93f"/>
    <rect x="100" y="12" width="520" height="24" rx="6" fill="#ffffff" stroke="${COLOR.border}" stroke-width="1"/>
    <text x="116" y="28" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}">chrome-extension://...  ·  Local Explorer</text>
    <rect x="1140" y="12" width="110" height="24" rx="4" fill="${COLOR.accentSoft}" stroke="${COLOR.accent}" stroke-width="1"/>
    <text x="1195" y="28" font-family="${FONT}" font-size="11" fill="${COLOR.accent}" text-anchor="middle" font-weight="600">Local Explorer</text>
  `;
}

// Reusable sidebar
function sidebar(activeIndex = 0) {
  const items = [
    { label: "C:\\", sub: "227 GB free" },
    { label: "D:\\", sub: "125 GB free" },
    { label: "E:\\", sub: "Removable" },
  ];
  let html = `<rect x="0" y="48" width="260" height="752" fill="${COLOR.sidebar}"/>
    <text x="24" y="84" font-family="${FONT}" font-size="11" font-weight="700" fill="${COLOR.textDim}" letter-spacing="1">DRIVES</text>`;
  items.forEach((it, i) => {
    const y = 100 + i * 64;
    const active = i === activeIndex;
    const bg = active ? COLOR.accent : COLOR.panel;
    const text = active ? "#ffffff" : COLOR.text;
    const sub = active ? "#cfe0f9" : COLOR.textDim;
    html += `
      <rect x="12" y="${y}" width="236" height="52" rx="8" fill="${bg}" stroke="${active ? COLOR.accentDark : COLOR.borderSoft}" stroke-width="1"/>
      ${driveIcon(24, y + 10)}
      <text x="72" y="${y + 24}" font-family="${FONT}" font-size="15" font-weight="600" fill="${text}">${it.label}</text>
      <text x="72" y="${y + 42}" font-family="${FONT}" font-size="11" fill="${sub}">${it.sub}</text>
    `;
  });

  html += `
    <text x="24" y="360" font-family="${FONT}" font-size="11" font-weight="700" fill="${COLOR.textDim}" letter-spacing="1">FAVORITES</text>
    ${[
      { y: 380, label: "Documents" },
      { y: 410, label: "Downloads" },
      { y: 440, label: "Pictures" },
    ].map((it) => `
      ${folderIcon(24, it.y + 2)}
      <text x="56" y="${it.y + 14}" font-family="${FONT}" font-size="13" fill="${COLOR.text}">${it.label}</text>
    `).join("")}
  `;
  return html;
}

// Toolbar (breadcrumb + actions)
function toolbar(crumbs, x = 260) {
  const trail = crumbs.map((c, i) => {
    const color = i === crumbs.length - 1 ? COLOR.text : COLOR.accent;
    const sep = i < crumbs.length - 1 ? `<text x="0" dx="8" dy="0" font-family="${FONT}" font-size="13" fill="${COLOR.textMuted}">›</text>` : "";
    return `<tspan fill="${color}" font-weight="${i === crumbs.length - 1 ? 600 : 400}">${c}</tspan>${i < crumbs.length - 1 ? ` <tspan fill="${COLOR.textMuted}"> › </tspan> ` : ""}`;
  }).join("");
  return `
    <rect x="${x}" y="48" width="${1280 - x}" height="56" fill="${COLOR.panel}" stroke="${COLOR.borderSoft}" stroke-width="1"/>
    <text x="${x + 24}" y="82" font-family="${FONT}" font-size="14" fill="${COLOR.text}">${trail}</text>
    <g transform="translate(${1280 - 300}, 60)">
      <rect x="0" y="0" width="72" height="32" rx="6" fill="${COLOR.panel}" stroke="${COLOR.border}"/>
      <text x="36" y="21" font-family="${FONT}" font-size="12" fill="${COLOR.text}" text-anchor="middle">새 폴더</text>
      <rect x="82" y="0" width="72" height="32" rx="6" fill="${COLOR.panel}" stroke="${COLOR.border}"/>
      <text x="118" y="21" font-family="${FONT}" font-size="12" fill="${COLOR.text}" text-anchor="middle">업로드</text>
      <rect x="164" y="0" width="110" height="32" rx="6" fill="${COLOR.accent}" stroke="${COLOR.accentDark}"/>
      <text x="219" y="21" font-family="${FONT}" font-size="12" fill="#ffffff" text-anchor="middle" font-weight="600">탐색기에서 보기</text>
    </g>
  `;
}

// Caption bar at bottom
function caption(title) {
  return `
    <rect x="0" y="758" width="1280" height="42" fill="${COLOR.accent}"/>
    <text x="640" y="786" font-family="${FONT}" font-size="16" font-weight="600" fill="#ffffff" text-anchor="middle">${title}</text>
  `;
}

// ---------- Screen 01: Drives home ----------
function screen01() {
  let svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800" viewBox="0 0 1280 800">
    <rect width="1280" height="800" fill="${COLOR.bg}"/>
    ${browserChrome()}
    ${sidebar(-1)}
    <rect x="260" y="48" width="1020" height="710" fill="${COLOR.panel}"/>
    <text x="300" y="120" font-family="${FONT}" font-size="28" font-weight="700" fill="${COLOR.text}">내 PC의 로컬 드라이브</text>
    <text x="300" y="150" font-family="${FONT}" font-size="14" fill="${COLOR.textDim}">탐색할 드라이브를 선택하세요. 네이티브 호스트를 통해 안전하게 접근합니다.</text>
  `;

  const drives = [
    { label: "C:\\", name: "Windows", free: "227 GB", total: "931 GB", fs: "NTFS · Fixed", pct: 0.76 },
    { label: "D:\\", name: "Data", free: "125 GB", total: "477 GB", fs: "NTFS · Fixed", pct: 0.74 },
    { label: "E:\\", name: "USB Drive", free: "42 GB", total: "64 GB", fs: "exFAT · Removable", pct: 0.34 },
  ];
  drives.forEach((d, i) => {
    const x = 300 + (i % 3) * 320;
    const y = 210;
    const barW = 260;
    const filled = Math.round(barW * d.pct);
    svg += `
      <g transform="translate(${x}, ${y})">
        <rect width="290" height="200" rx="12" fill="${COLOR.panel}" stroke="${COLOR.border}" stroke-width="1.5"/>
        ${driveIcon(24, 24)}
        <text x="72" y="42" font-family="${FONT}" font-size="22" font-weight="700" fill="${COLOR.text}">${d.label}</text>
        <text x="72" y="62" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}">${d.name}</text>
        <text x="24" y="102" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}">${d.fs}</text>

        <rect x="24" y="118" width="${barW}" height="8" rx="4" fill="${COLOR.borderSoft}"/>
        <rect x="24" y="118" width="${filled}" height="8" rx="4" fill="${d.pct > 0.85 ? COLOR.danger : COLOR.accent}"/>

        <text x="24" y="148" font-family="${FONT}" font-size="12" fill="${COLOR.text}" font-weight="600">${d.free} free</text>
        <text x="${24 + barW}" y="148" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}" text-anchor="end">of ${d.total}</text>

        <rect x="24" y="160" width="90" height="26" rx="5" fill="${COLOR.accentSoft}"/>
        <text x="69" y="177" font-family="${FONT}" font-size="11" font-weight="600" fill="${COLOR.accent}" text-anchor="middle">열기</text>
      </g>
    `;
  });

  svg += `
    <g transform="translate(300, 470)">
      <rect width="930" height="110" rx="10" fill="${COLOR.accentSoft}" stroke="${COLOR.accent}" stroke-width="1"/>
      <text x="24" y="36" font-family="${FONT}" font-size="14" font-weight="700" fill="${COLOR.accentDark}">네이티브 호스트 연결됨</text>
      <text x="24" y="60" font-family="${FONT}" font-size="12" fill="${COLOR.text}">com.local.fx · v0.2.0 · 허용된 앱 ID로 연결 (Windows)</text>
      <text x="24" y="82" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}">드라이브 나열 · 파일 읽기/쓰기 · 복사/이동/삭제 · 스트리밍 전송 · 충돌 처리</text>
    </g>
    ${caption("로컬 드라이브 탐색 — 네이티브 메시징 호스트로 안전하게 접근")}
  </svg>`;
  return svg;
}

// ---------- Screen 02: File list (sort + selection) ----------
function screen02() {
  const rows = [
    { type: "dir", name: "System32", size: "—", date: "2026-04-20 09:14" },
    { type: "dir", name: "Program Files", size: "—", date: "2026-04-18 22:41" },
    { type: "dir", name: "Users", size: "—", date: "2026-04-22 07:03" },
    { type: "dir", name: "Windows", size: "—", date: "2026-04-23 10:28" },
    { type: "file", name: "bcdedit.log", size: "12 KB", date: "2026-04-22 14:50" },
    { type: "file", name: "explorer.exe", size: "3.0 MB", date: "2026-04-08 11:02" },
    { type: "file", name: "notepad.exe", size: "196 KB", date: "2026-03-29 18:10" },
    { type: "file", name: "regedit.exe", size: "420 KB", date: "2026-04-01 00:00" },
    { type: "file", name: "setup.log", size: "88 KB", date: "2026-04-17 06:33" },
    { type: "file", name: "WindowsUpdate.log", size: "1.2 MB", date: "2026-04-23 09:45" },
    { type: "file", name: "dism.log", size: "340 KB", date: "2026-04-21 11:29" },
    { type: "file", name: "license.rtf", size: "28 KB", date: "2026-02-14 12:00" },
    { type: "file", name: "readme.txt", size: "4 KB", date: "2026-01-10 09:00" },
  ];
  const selected = new Set([2, 6, 9]);

  let rowsSVG = "";
  rows.forEach((r, i) => {
    const y = 164 + i * 40;
    const isSel = selected.has(i);
    const bg = isSel ? COLOR.rowSel : (i % 2 ? COLOR.rowAlt : COLOR.row);
    const icon = r.type === "dir" ? folderIcon(326, y + 11) : fileIcon(327, y + 10);
    rowsSVG += `
      <rect x="260" y="${y}" width="1020" height="40" fill="${bg}"/>
      <rect x="268" y="${y + 13}" width="14" height="14" rx="3" fill="${isSel ? COLOR.accent : COLOR.panel}" stroke="${isSel ? COLOR.accentDark : COLOR.border}"/>
      ${isSel ? `<path d="M 270 ${y + 20} L 274 ${y + 24} L 280 ${y + 16}" stroke="#ffffff" stroke-width="2" fill="none"/>` : ""}
      ${icon}
      <text x="360" y="${y + 26}" font-family="${FONT}" font-size="13" fill="${COLOR.text}">${r.name}</text>
      <text x="900" y="${y + 26}" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}" text-anchor="end">${r.size}</text>
      <text x="1250" y="${y + 26}" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}" text-anchor="end">${r.date}</text>
    `;
  });

  return `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800" viewBox="0 0 1280 800">
    <rect width="1280" height="800" fill="${COLOR.bg}"/>
    ${browserChrome()}
    ${sidebar(0)}
    ${toolbar(["Drives", "C:\\", "Windows"])}

    <!-- Header row -->
    <rect x="260" y="120" width="1020" height="40" fill="${COLOR.header}" stroke="${COLOR.borderSoft}" stroke-width="1"/>
    <text x="300" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}">이름  ▲</text>
    <text x="900" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}" text-anchor="end">크기</text>
    <text x="1250" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}" text-anchor="end">수정일</text>

    ${rowsSVG}

    <!-- Status bar -->
    <rect x="260" y="720" width="1020" height="38" fill="${COLOR.header}" stroke="${COLOR.borderSoft}" stroke-width="1"/>
    <text x="284" y="744" font-family="${FONT}" font-size="12" fill="${COLOR.text}">13개 항목 · 3개 선택됨 · 크기 합계 1.54 MB</text>
    <text x="1256" y="744" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}" text-anchor="end">페이지 1/8</text>

    ${caption("파일 리스트 — 정렬 가능한 컬럼과 다중 선택")}
  </svg>`;
}

// ---------- Screen 03: Context menu ----------
function screen03() {
  const rows = [
    { type: "dir", name: "photos", size: "—", date: "2026-04-22 10:04" },
    { type: "dir", name: "videos", size: "—", date: "2026-04-20 17:38" },
    { type: "dir", name: "archive", size: "—", date: "2026-03-15 08:12" },
    { type: "file", name: "backup_2026-04-23.zip", size: "1.8 GB", date: "2026-04-23 22:00" },
    { type: "file", name: "quarterly-report.xlsx", size: "2.4 MB", date: "2026-04-21 15:44" },
    { type: "file", name: "design-notes.md", size: "18 KB", date: "2026-04-22 09:12" },
    { type: "file", name: "build-log.txt", size: "340 KB", date: "2026-04-23 11:28" },
    { type: "file", name: "screenshot.png", size: "980 KB", date: "2026-04-24 09:15" },
    { type: "file", name: "invoice.pdf", size: "156 KB", date: "2026-04-10 14:00" },
  ];
  const highlightedIndex = 4;

  let rowsSVG = "";
  rows.forEach((r, i) => {
    const y = 164 + i * 40;
    const isHi = i === highlightedIndex;
    const bg = isHi ? COLOR.rowSel : (i % 2 ? COLOR.rowAlt : COLOR.row);
    const icon = r.type === "dir" ? folderIcon(326, y + 11) : fileIcon(327, y + 10);
    rowsSVG += `
      <rect x="260" y="${y}" width="1020" height="40" fill="${bg}"/>
      ${icon}
      <text x="360" y="${y + 26}" font-family="${FONT}" font-size="13" fill="${COLOR.text}">${r.name}</text>
      <text x="900" y="${y + 26}" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}" text-anchor="end">${r.size}</text>
      <text x="1250" y="${y + 26}" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}" text-anchor="end">${r.date}</text>
    `;
  });

  // Context menu anchored near the highlighted row
  const menuX = 520;
  const menuY = 330;
  const menuItems = [
    { label: "열기", shortcut: "Enter", color: COLOR.text },
    { label: "탐색기에서 보기", shortcut: "", color: COLOR.text },
    { sep: true },
    { label: "복사", shortcut: "Ctrl+C", color: COLOR.text },
    { label: "잘라내기", shortcut: "Ctrl+X", color: COLOR.text },
    { label: "붙여넣기", shortcut: "Ctrl+V", color: COLOR.text },
    { sep: true },
    { label: "이름 변경", shortcut: "F2", color: COLOR.text },
    { label: "휴지통으로 이동", shortcut: "Del", color: COLOR.danger },
    { label: "영구 삭제", shortcut: "Shift+Del", color: COLOR.danger },
    { sep: true },
    { label: "속성", shortcut: "Alt+Enter", color: COLOR.text },
  ];

  let menuSVG = `<g transform="translate(${menuX}, ${menuY})">`;
  // Shadow
  menuSVG += `<rect x="4" y="6" width="280" height="358" rx="8" fill="#000000" opacity="0.12"/>`;
  menuSVG += `<rect x="0" y="0" width="280" height="358" rx="8" fill="${COLOR.panel}" stroke="${COLOR.border}" stroke-width="1"/>`;

  let cursor = 10;
  menuItems.forEach((it, idx) => {
    if (it.sep) {
      menuSVG += `<line x1="12" y1="${cursor + 6}" x2="268" y2="${cursor + 6}" stroke="${COLOR.borderSoft}"/>`;
      cursor += 12;
    } else {
      const hover = idx === 3; // "복사" hover
      if (hover) {
        menuSVG += `<rect x="6" y="${cursor}" width="268" height="28" rx="4" fill="${COLOR.accentSoft}"/>`;
      }
      menuSVG += `<text x="20" y="${cursor + 19}" font-family="${FONT}" font-size="13" fill="${it.color}">${it.label}</text>`;
      if (it.shortcut) {
        menuSVG += `<text x="264" y="${cursor + 19}" font-family="${FONT}" font-size="11" fill="${COLOR.textMuted}" text-anchor="end">${it.shortcut}</text>`;
      }
      cursor += 28;
    }
  });
  menuSVG += `</g>`;

  return `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800" viewBox="0 0 1280 800">
    <rect width="1280" height="800" fill="${COLOR.bg}"/>
    ${browserChrome()}
    ${sidebar(1)}
    ${toolbar(["Drives", "D:\\", "work"])}

    <!-- Header row -->
    <rect x="260" y="120" width="1020" height="40" fill="${COLOR.header}" stroke="${COLOR.borderSoft}" stroke-width="1"/>
    <text x="300" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}">이름</text>
    <text x="900" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}" text-anchor="end">크기</text>
    <text x="1250" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}" text-anchor="end">수정일</text>

    ${rowsSVG}

    ${menuSVG}

    ${caption("우클릭 메뉴 — 파일 조작과 키보드 단축키 지원")}
  </svg>`;
}

// ---------- Screen 04: Progress toast ----------
function screen04() {
  const rows = [
    { type: "dir", name: "photos (복사 중)", size: "—", date: "2026-04-24 09:40" },
    { type: "file", name: "IMG_4198.jpg", size: "14.2 MB", date: "2026-04-14 11:20" },
    { type: "file", name: "IMG_4199.jpg", size: "14.5 MB", date: "2026-04-14 11:21" },
    { type: "file", name: "IMG_4200.jpg", size: "13.8 MB", date: "2026-04-14 11:22" },
    { type: "file", name: "IMG_4201.jpg", size: "15.1 MB", date: "2026-04-14 11:22" },
    { type: "file", name: "IMG_4202.jpg", size: "14.0 MB", date: "2026-04-14 11:23" },
    { type: "file", name: "IMG_4203.jpg", size: "14.4 MB", date: "2026-04-14 11:23" },
    { type: "file", name: "IMG_4204.jpg", size: "13.9 MB", date: "2026-04-14 11:24" },
    { type: "file", name: "IMG_4205.jpg", size: "14.2 MB", date: "2026-04-14 11:24" },
  ];

  let rowsSVG = "";
  rows.forEach((r, i) => {
    const y = 164 + i * 40;
    const bg = i % 2 ? COLOR.rowAlt : COLOR.row;
    const icon = r.type === "dir" ? folderIcon(326, y + 11) : fileIcon(327, y + 10);
    rowsSVG += `
      <rect x="260" y="${y}" width="1020" height="40" fill="${bg}"/>
      ${icon}
      <text x="360" y="${y + 26}" font-family="${FONT}" font-size="13" fill="${COLOR.text}">${r.name}</text>
      <text x="900" y="${y + 26}" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}" text-anchor="end">${r.size}</text>
      <text x="1250" y="${y + 26}" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}" text-anchor="end">${r.date}</text>
    `;
  });

  // Progress toasts (bottom-right stack)
  const toast = (x, y, data) => `
    <g transform="translate(${x}, ${y})">
      <rect x="4" y="6" width="${data.w}" height="${data.h}" rx="10" fill="#000000" opacity="0.10"/>
      <rect x="0" y="0" width="${data.w}" height="${data.h}" rx="10" fill="${COLOR.panel}" stroke="${data.accent || COLOR.accent}" stroke-width="1.5"/>
      ${data.inner}
    </g>
  `;

  const activeToast = toast(880, 440, {
    w: 380,
    h: 150,
    inner: `
      <text x="20" y="32" font-family="${FONT}" font-size="14" font-weight="700" fill="${COLOR.text}">복사 중 · photos/</text>
      <text x="360" y="32" font-family="${FONT}" font-size="14" font-weight="700" fill="${COLOR.accent}" text-anchor="end">42%</text>
      <rect x="20" y="48" width="340" height="6" rx="3" fill="${COLOR.borderSoft}"/>
      <rect x="20" y="48" width="143" height="6" rx="3" fill="${COLOR.accent}"/>
      <text x="20" y="78" font-family="${FONT}" font-size="11" fill="${COLOR.text}">420.3 MB / 1.00 GB · 파일 42 / 100 · 28.5 MB/s</text>
      <text x="20" y="96" font-family="${FONT}" font-size="11" fill="${COLOR.textDim}">D:\\backup\\photos\\IMG_4203.jpg</text>
      <rect x="20" y="110" width="68" height="28" rx="5" fill="${COLOR.panel}" stroke="${COLOR.border}"/>
      <text x="54" y="128" font-family="${FONT}" font-size="12" fill="${COLOR.text}" text-anchor="middle">일시중지</text>
      <rect x="96" y="110" width="58" height="28" rx="5" fill="${COLOR.panel}" stroke="${COLOR.danger}"/>
      <text x="125" y="128" font-family="${FONT}" font-size="12" fill="${COLOR.danger}" text-anchor="middle">취소</text>
      <text x="360" y="128" font-family="${FONT}" font-size="11" fill="${COLOR.textDim}" text-anchor="end">남은 시간 약 21초</text>
    `,
  });

  const queuedToast = toast(880, 600, {
    w: 380,
    h: 64,
    accent: COLOR.warning,
    inner: `
      <text x="20" y="28" font-family="${FONT}" font-size="13" font-weight="700" fill="${COLOR.text}">대기 중 · videos/</text>
      <text x="360" y="28" font-family="${FONT}" font-size="12" fill="${COLOR.warning}" text-anchor="end">큐에서 대기</text>
      <text x="20" y="48" font-family="${FONT}" font-size="11" fill="${COLOR.textDim}">2.4 GB · 파일 58개 · 동시 전송 한도 1개</text>
    `,
  });

  const doneToast = toast(880, 680, {
    w: 380,
    h: 64,
    accent: COLOR.success,
    inner: `
      <text x="20" y="28" font-family="${FONT}" font-size="13" font-weight="700" fill="${COLOR.text}">완료 · report.pdf</text>
      <text x="360" y="28" font-family="${FONT}" font-size="12" fill="${COLOR.success}" text-anchor="end">✓ 성공</text>
      <text x="20" y="48" font-family="${FONT}" font-size="11" fill="${COLOR.textDim}">2.4 MB 전송 · 0.3초 경과</text>
    `,
  });

  return `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800" viewBox="0 0 1280 800">
    <rect width="1280" height="800" fill="${COLOR.bg}"/>
    ${browserChrome()}
    ${sidebar(1)}
    ${toolbar(["Drives", "D:\\", "backup"])}

    <rect x="260" y="120" width="1020" height="40" fill="${COLOR.header}" stroke="${COLOR.borderSoft}" stroke-width="1"/>
    <text x="300" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}">이름</text>
    <text x="900" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}" text-anchor="end">크기</text>
    <text x="1250" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}" text-anchor="end">수정일</text>

    ${rowsSVG}

    ${activeToast}
    ${queuedToast}
    ${doneToast}

    ${caption("진행률 토스트 — 실시간 속도 · 파일 수 · 남은 시간 · 취소 지원")}
  </svg>`;
}

// ---------- Screen 05: Conflict dialog ----------
function screen05() {
  // Dimmed background list
  const rows = [
    { type: "dir", name: "source" },
    { type: "dir", name: "target" },
    { type: "file", name: "project_notes.md" },
    { type: "file", name: "README.md" },
    { type: "file", name: "todo.txt" },
  ];
  let rowsSVG = "";
  rows.forEach((r, i) => {
    const y = 164 + i * 40;
    const bg = i % 2 ? COLOR.rowAlt : COLOR.row;
    const icon = r.type === "dir" ? folderIcon(326, y + 11) : fileIcon(327, y + 10);
    rowsSVG += `
      <rect x="260" y="${y}" width="1020" height="40" fill="${bg}"/>
      ${icon}
      <text x="360" y="${y + 26}" font-family="${FONT}" font-size="13" fill="${COLOR.textDim}">${r.name}</text>
    `;
  });

  // Dialog
  const dialogX = 310;
  const dialogY = 150;
  const dialogW = 660;
  const dialogH = 500;

  const dialog = `
    <!-- modal scrim -->
    <rect x="0" y="48" width="1280" height="710" fill="#0b1220" opacity="0.45"/>

    <g transform="translate(${dialogX}, ${dialogY})">
      <rect x="6" y="10" width="${dialogW}" height="${dialogH}" rx="12" fill="#000000" opacity="0.18"/>
      <rect x="0" y="0" width="${dialogW}" height="${dialogH}" rx="12" fill="${COLOR.panel}" stroke="${COLOR.border}"/>

      <!-- header -->
      <rect x="0" y="0" width="${dialogW}" height="64" rx="12" fill="${COLOR.warningSoft}"/>
      <rect x="0" y="54" width="${dialogW}" height="10" fill="${COLOR.warningSoft}"/>
      <circle cx="40" cy="32" r="16" fill="${COLOR.warning}"/>
      <text x="40" y="38" font-family="${FONT}" font-size="20" font-weight="900" fill="#ffffff" text-anchor="middle">!</text>
      <text x="72" y="30" font-family="${FONT}" font-size="16" font-weight="700" fill="${COLOR.text}">대상 이름이 이미 존재합니다</text>
      <text x="72" y="50" font-family="${FONT}" font-size="12" fill="${COLOR.textDim}">남은 충돌 3건 · 해결 방법을 선택하세요</text>

      <!-- compare panels -->
      <g transform="translate(24, 84)">
        <rect width="296" height="160" rx="8" fill="${COLOR.bg}" stroke="${COLOR.border}"/>
        <text x="16" y="24" font-family="${FONT}" font-size="11" font-weight="700" fill="${COLOR.textDim}" letter-spacing="1">원본 (SOURCE)</text>
        ${fileIcon(16, 38)}
        <text x="44" y="54" font-family="${FONT}" font-size="13" font-weight="600" fill="${COLOR.text}">project_notes.md</text>
        <text x="16" y="88" font-family="${FONT}" font-size="11" fill="${COLOR.textDim}">경로</text>
        <text x="16" y="104" font-family="'Consolas', monospace" font-size="12" fill="${COLOR.text}">D:\\source\\docs\\</text>
        <text x="16" y="128" font-family="${FONT}" font-size="11" fill="${COLOR.textDim}">크기 18.4 KB  ·  수정 2026-04-24 09:12</text>
        <rect x="16" y="138" width="70" height="18" rx="3" fill="${COLOR.accentSoft}"/>
        <text x="51" y="151" font-family="${FONT}" font-size="10" fill="${COLOR.accent}" text-anchor="middle" font-weight="700">최신</text>
      </g>
      <g transform="translate(340, 84)">
        <rect width="296" height="160" rx="8" fill="${COLOR.warningSoft}" stroke="${COLOR.warningBorder}"/>
        <text x="16" y="24" font-family="${FONT}" font-size="11" font-weight="700" fill="#b86c00" letter-spacing="1">기존 대상 (TARGET)</text>
        ${fileIcon(16, 38)}
        <text x="44" y="54" font-family="${FONT}" font-size="13" font-weight="600" fill="${COLOR.text}">project_notes.md</text>
        <text x="16" y="88" font-family="${FONT}" font-size="11" fill="${COLOR.textDim}">경로</text>
        <text x="16" y="104" font-family="'Consolas', monospace" font-size="12" fill="${COLOR.text}">D:\\target\\docs\\</text>
        <text x="16" y="128" font-family="${FONT}" font-size="11" fill="${COLOR.textDim}">크기 14.1 KB  ·  수정 2026-04-20 15:44</text>
        <rect x="16" y="138" width="70" height="18" rx="3" fill="#fde3bd"/>
        <text x="51" y="151" font-family="${FONT}" font-size="10" fill="#8a5400" text-anchor="middle" font-weight="700">이전 버전</text>
      </g>

      <!-- Rename preview -->
      <g transform="translate(24, 262)">
        <text x="0" y="14" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}">자동 이름 변경 미리보기</text>
        <rect x="0" y="24" width="612" height="36" rx="6" fill="${COLOR.bg}" stroke="${COLOR.border}"/>
        <text x="14" y="47" font-family="'Consolas', monospace" font-size="13" fill="${COLOR.text}">project_notes (2).md</text>
        <text x="598" y="47" font-family="${FONT}" font-size="11" fill="${COLOR.textDim}" text-anchor="end">다음 사용 가능한 번호</text>
      </g>

      <!-- apply-to-all checkbox -->
      <g transform="translate(24, 324)">
        <rect x="0" y="0" width="18" height="18" rx="4" fill="${COLOR.accent}" stroke="${COLOR.accentDark}"/>
        <path d="M 4 9 L 8 13 L 14 5" stroke="#ffffff" stroke-width="2" fill="none"/>
        <text x="28" y="14" font-family="${FONT}" font-size="13" fill="${COLOR.text}">이후 충돌 3건에도 동일하게 적용</text>
      </g>

      <!-- divider -->
      <line x1="0" y1="370" x2="${dialogW}" y2="370" stroke="${COLOR.borderSoft}"/>

      <!-- buttons -->
      <g transform="translate(24, 400)">
        <rect x="0" y="0" width="100" height="40" rx="6" fill="${COLOR.panel}" stroke="${COLOR.border}"/>
        <text x="50" y="25" font-family="${FONT}" font-size="13" fill="${COLOR.text}" text-anchor="middle">취소</text>
      </g>
      <g transform="translate(${dialogW - 24 - 480}, 400)">
        <rect x="0" y="0" width="150" height="40" rx="6" fill="${COLOR.panel}" stroke="${COLOR.border}"/>
        <text x="75" y="25" font-family="${FONT}" font-size="13" font-weight="600" fill="${COLOR.text}" text-anchor="middle">건너뛰기</text>

        <rect x="160" y="0" width="150" height="40" rx="6" fill="${COLOR.danger}"/>
        <text x="235" y="25" font-family="${FONT}" font-size="13" font-weight="700" fill="#ffffff" text-anchor="middle">덮어쓰기</text>

        <rect x="320" y="0" width="160" height="40" rx="6" fill="${COLOR.accent}"/>
        <text x="400" y="25" font-family="${FONT}" font-size="13" font-weight="700" fill="#ffffff" text-anchor="middle">이름 변경 (기본)</text>
      </g>
    </g>
  `;

  return `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800" viewBox="0 0 1280 800">
    <rect width="1280" height="800" fill="${COLOR.bg}"/>
    ${browserChrome()}
    ${sidebar(1)}
    ${toolbar(["Drives", "D:\\", "target"])}

    <rect x="260" y="120" width="1020" height="40" fill="${COLOR.header}" stroke="${COLOR.borderSoft}" stroke-width="1"/>
    <text x="300" y="145" font-family="${FONT}" font-size="12" font-weight="700" fill="${COLOR.text}">이름</text>

    ${rowsSVG}
    ${dialog}

    ${caption("충돌 해결 — 원본/대상 비교, 덮어쓰기·건너뛰기·이름변경")}
  </svg>`;
}

// ---------- Render all ----------
const screens = [
  { name: "01-drives.png", svg: screen01() },
  { name: "02-filelist.png", svg: screen02() },
  { name: "03-context-menu.png", svg: screen03() },
  { name: "04-progress.png", svg: screen04() },
  { name: "05-conflict.png", svg: screen05() },
];

const results = [];
for (const s of screens) {
  const resvg = new Resvg(s.svg, {
    background: "#ffffff",
    fitTo: { mode: "width", value: 1280 },
    font: { loadSystemFonts: true },
  });
  const pngData = resvg.render();
  const png = pngData.asPng();
  const outPath = path.join(outDir, s.name);
  fs.writeFileSync(outPath, png);
  const dims = pngData;
  results.push({
    name: s.name,
    path: outPath,
    bytes: png.length,
    width: dims.width,
    height: dims.height,
  });
  console.log(`  ${s.name} -> ${png.length} bytes (${dims.width}x${dims.height})`);
}

console.log("\nSummary:");
for (const r of results) {
  console.log(`  ${r.path}  ${r.width}x${r.height}  ${(r.bytes / 1024).toFixed(1)} KB`);
}
