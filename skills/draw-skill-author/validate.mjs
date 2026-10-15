#!/usr/bin/env node
// Validate a Draw Studio operator skill package directory before zipping it.
// Usage: node validate.mjs <slug-dir>
// Mirrors the server-side checks in format.md (draw-platform-skill/1). Node 18+, no dependencies.
// Exit code 0 = all checks passed, 1 = problems found (all of them are listed).

import { readFileSync, readdirSync, statSync, existsSync } from "node:fs";
import { join, extname, basename } from "node:path";

const FORMAT = "draw-platform-skill/1";
// Keep in sync with format.md §3 (the site's gate test compares the two).
const CATEGORIES = ["social", "ecommerce", "design", "academic", "writing", "other"];
const IMAGE_EXTS = new Set([".png", ".jpg", ".jpeg", ".webp"]);
const LIMITS = {
  slug: 40,
  name: 30,
  summary: 200,
  placeholder: 120,
  body: 20000,
  keywords: 5,
  keyword: 20,
  logo: 20000,
  note: 60,
  examples: 6,
  references: 4,
  image: 5 * 1024 * 1024,
};

const problems = [];
const fail = (field, msg) => problems.push(`${field}: ${msg}`);
const runes = (s) => [...s].length;

// --- minimal YAML subset (format.md §2 "写法限制") --------------------------

function parseQuoted(src, i) {
  // src[i] === '"'
  let out = "";
  i++;
  while (i < src.length) {
    const c = src[i];
    if (c === "\\") {
      const n = src[i + 1];
      out += n === "n" ? "\n" : n === "t" ? "\t" : n;
      i += 2;
      continue;
    }
    if (c === '"') return [out, i + 1];
    out += c;
    i++;
  }
  throw new Error("unterminated quoted string");
}

function parseScalar(src, i, stops) {
  while (src[i] === " ") i++;
  if (src[i] === '"') return parseQuoted(src, i);
  let j = i;
  while (j < src.length && !stops.includes(src[j])) j++;
  return [src.slice(i, j).trim(), j];
}

function parseFlowMap(src) {
  const s = src.trim();
  if (!s.startsWith("{") || !s.endsWith("}")) throw new Error(`expected { … }, got ${s}`);
  const body = s.slice(1, -1);
  const out = {};
  let i = 0;
  while (i < body.length) {
    while (body[i] === " " || body[i] === ",") i++;
    if (i >= body.length) break;
    let key;
    [key, i] = parseScalar(body, i, [":"]);
    if (body[i] !== ":") throw new Error(`missing ':' after ${key}`);
    i++;
    let val;
    [val, i] = parseScalar(body, i, [","]);
    out[key] = val;
  }
  return out;
}

function parseFlowSeq(src) {
  const s = src.trim();
  if (!s.startsWith("[") || !s.endsWith("]")) throw new Error(`expected [ … ], got ${s}`);
  const body = s.slice(1, -1);
  const out = [];
  let i = 0;
  while (i < body.length) {
    while (body[i] === " " || body[i] === ",") i++;
    if (i >= body.length) break;
    let val;
    [val, i] = parseScalar(body, i, [","]);
    out.push(val);
  }
  return out;
}

function parseFrontmatter(text) {
  const lines = text.split("\n");
  const out = {};
  for (let n = 0; n < lines.length; n++) {
    const line = lines[n];
    if (!line.trim() || line.trim().startsWith("#")) continue;
    const m = /^([A-Za-z_][\w-]*):\s?(.*)$/.exec(line);
    if (!m) throw new Error(`line ${n + 1}: expected "key: value", got ${line}`);
    const [, key, rest] = m;
    const v = rest.trim();
    if (v === "|-" || v === "|") {
      const block = [];
      while (n + 1 < lines.length && (lines[n + 1].startsWith("  ") || lines[n + 1] === "")) {
        block.push(lines[++n].slice(2));
      }
      out[key] = block.join("\n").trim();
    } else if (v === "") {
      const items = [];
      while (n + 1 < lines.length && /^\s+-\s/.test(lines[n + 1])) {
        items.push(parseFlowMap(lines[++n].replace(/^\s+-\s/, "")));
      }
      out[key] = items;
    } else if (v.startsWith("{")) {
      out[key] = parseFlowMap(v);
    } else if (v.startsWith("[")) {
      out[key] = parseFlowSeq(v);
    } else {
      out[key] = parseScalar(v, 0, [])[0];
    }
  }
  return out;
}

// --- checks -----------------------------------------------------------------

function single(field, value, max, required) {
  const v = (value ?? "").trim();
  if (!v) {
    if (required) fail(field, "必填");
    return;
  }
  if (runes(v) > max) fail(field, `最长 ${max} 字(现在 ${runes(v)})`);
  if (/[\r\n]/.test(v)) fail(field, "不能换行");
}

function sniff(buf) {
  if (buf.length >= 8 && buf[0] === 0x89 && buf[1] === 0x50 && buf[2] === 0x4e && buf[3] === 0x47) return "png";
  if (buf.length >= 3 && buf[0] === 0xff && buf[1] === 0xd8 && buf[2] === 0xff) return "jpeg";
  if (buf.length >= 12 && buf.toString("ascii", 0, 4) === "RIFF" && buf.toString("ascii", 8, 12) === "WEBP") return "webp";
  return "";
}

function main() {
  const dir = process.argv[2];
  if (!dir) {
    console.error("用法: node validate.mjs <技能目录>");
    process.exit(2);
  }
  const skillPath = join(dir, "SKILL.md");
  if (!existsSync(skillPath)) {
    console.error(`找不到 ${skillPath}`);
    process.exit(1);
  }
  const text = readFileSync(skillPath, "utf8").replace(/\r\n/g, "\n").replace(/^﻿/, "");
  const m = /^---\n([\s\S]*?)\n---[^\n]*\n?([\s\S]*)$/.exec(text);
  if (!m) {
    fail("SKILL.md", "开头需要 --- 包起来的 frontmatter");
    return report();
  }
  let fm;
  try {
    fm = parseFrontmatter(m[1]);
  } catch (e) {
    fail("frontmatter", e.message);
    return report();
  }
  const body = m[2].trim();

  if ((fm.format ?? "").trim() !== FORMAT) fail("format", `必须是 ${FORMAT}`);
  const slug = (fm.slug ?? basename(dir)).trim();
  if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(slug) || slug.length > LIMITS.slug) {
    fail("slug", `只能用小写字母、数字和连字符,最长 ${LIMITS.slug} 个字符`);
  }
  for (const [key, max, required] of [
    ["name", LIMITS.name, true],
    ["summary", LIMITS.summary, true],
    ["placeholder", LIMITS.placeholder, false],
  ]) {
    const pair = fm[key];
    if (pair !== undefined && (typeof pair !== "object" || Array.isArray(pair))) {
      fail(key, '要写成 { zh: "…", en: "…" }');
      continue;
    }
    single(`${key}.zh`, pair?.zh, max, required);
    single(`${key}.en`, pair?.en, max, required);
  }
  if (!CATEGORIES.includes((fm.category ?? "").trim())) fail("category", `只能是 ${CATEGORIES.join(" / ")}`);
  const kws = fm.keywords ?? [];
  if (!Array.isArray(kws)) fail("keywords", "要写成 [a, b, c]");
  else {
    if (kws.length > LIMITS.keywords) fail("keywords", `最多 ${LIMITS.keywords} 个`);
    for (const k of kws) if (runes(k) > LIMITS.keyword || /[,\n]/.test(k)) fail("keywords", `每个最长 ${LIMITS.keyword} 字,不含逗号`);
  }
  if (fm.logo && (!fm.logo.startsWith("<svg") || Buffer.byteLength(fm.logo) > LIMITS.logo)) {
    fail("logo", `需以 <svg 开头,最大 ${LIMITS.logo} 字节`);
  }
  if (!body) fail("正文", "必填");
  else if (runes(body) > LIMITS.body) fail("正文", `最长 ${LIMITS.body} 字(现在 ${runes(body)})`);
  else if (body.startsWith("---")) fail("正文", "不能再以 --- 开头");

  const images = (sub) => {
    const p = join(dir, sub);
    if (!existsSync(p)) return [];
    return readdirSync(p)
      .filter((f) => !f.startsWith("."))
      .sort();
  };
  const checkImages = (sub, max) => {
    const files = images(sub);
    const imgs = files.filter((f) => IMAGE_EXTS.has(extname(f).toLowerCase()));
    for (const f of files) if (!IMAGE_EXTS.has(extname(f).toLowerCase())) fail(`${sub}/${f}`, "不是 PNG / JPEG / WebP,会被忽略");
    if (imgs.length > max) fail(sub, `最多 ${max} 张`);
    for (const f of imgs) {
      const path = join(dir, sub, f);
      if (statSync(path).size > LIMITS.image) fail(`${sub}/${f}`, "单张不能超过 5 MB");
      if (!sniff(readFileSync(path))) fail(`${sub}/${f}`, "文件内容不是 PNG / JPEG / WebP");
    }
    return imgs;
  };
  checkImages("examples", LIMITS.examples);
  const refFiles = checkImages("references", LIMITS.references);
  const refs = fm.references ?? [];
  if (!Array.isArray(refs)) fail("references", "要写成每项一行的列表");
  else {
    for (const r of refs) {
      if (!r.file) fail("references", "每项要有 file");
      else if (!refFiles.includes(basename(r.file))) fail("references", `references/ 里没有 ${r.file}`);
      if (!(r.note ?? "").trim()) fail("references", `${r.file} 没写 note`);
      else if (runes(r.note) > LIMITS.note) fail("references", `${r.file} 的 note 最长 ${LIMITS.note} 字`);
    }
    for (const f of refFiles) if (!refs.some((r) => basename(r.file ?? "") === f)) fail("references", `${f} 没写 note`);
  }
  return report();
}

function report() {
  if (problems.length) {
    console.error(`未通过(${problems.length} 项):`);
    for (const p of problems) console.error(`- ${p}`);
    process.exit(1);
  }
  console.log("通过。可以打包上传。");
}

main();
