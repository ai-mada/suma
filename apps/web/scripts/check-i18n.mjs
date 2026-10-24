import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import crypto from "node:crypto";
import ts from "typescript";
const base = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const args = process.argv.slice(2),
  option = (name) => {
    const index = args.indexOf(name);
    return index < 0 ? undefined : args[index + 1];
  };
const sourceRoot = path.join(base, "src"),
  catalogDir = path.resolve(
    option("--catalog-dir") || path.join(sourceRoot, "i18n/locales"),
  ),
  baselinePath = path.resolve(
    option("--baseline") || path.join(sourceRoot, "i18n/legacy-baseline.json"),
  );
const errors = [];
function catalog(locale) {
  const file = path.join(catalogDir, locale + ".json"),
    raw = fs.readFileSync(file, "utf8"),
    seen = new Set();
  for (const match of raw.matchAll(/"(?:[^"\\]|\\.)*"/g)) {
    if (!/^\s*:/.test(raw.slice(match.index + match[0].length))) continue;
    const key = JSON.parse(match[0]);
    if (seen.has(key)) errors.push("duplicate key " + key);
    seen.add(key);
  }
  const value = JSON.parse(raw);
  if (!value || Array.isArray(value) || typeof value !== "object")
    throw Error("catalog must be an object");
  for (const [key, text] of Object.entries(value))
    if (typeof text !== "string" || !text.trim() || text.includes("\uFFFD"))
      errors.push("empty or invalid value " + key);
  return value;
}
function files(root) {
  return fs
    .readdirSync(root, { withFileTypes: true })
    .flatMap((entry) =>
      entry.isDirectory()
        ? files(path.join(root, entry.name))
        : /\.(?:ts|tsx)$/.test(entry.name) && !entry.name.includes(".test.")
          ? [path.join(root, entry.name)]
          : [],
    );
}
const sourceFiles = args.includes("--skip-audit") ? [] : files(sourceRoot),
  allowedTags = new Map();
for (const file of sourceFiles) {
  const sf = ts.createSourceFile(
    file,
    fs.readFileSync(file, "utf8"),
    ts.ScriptTarget.Latest,
    true,
    file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  function visit(node) {
    if (
      (ts.isJsxSelfClosingElement(node) || ts.isJsxOpeningElement(node)) &&
      node.tagName.getText(sf) === "Trans"
    ) {
      const attrs = node.attributes.properties,
        key = attrs.find(
          (item) =>
            ts.isJsxAttribute(item) && item.name.getText(sf) === "i18nKey",
        ),
        components = attrs.find(
          (item) =>
            ts.isJsxAttribute(item) && item.name.getText(sf) === "components",
        );
      if (key?.initializer && ts.isStringLiteral(key.initializer)) {
        const tags = allowedTags.get(key.initializer.text) || new Set(["br"]);
        if (
          components?.initializer &&
          ts.isJsxExpression(components.initializer) &&
          components.initializer.expression &&
          ts.isObjectLiteralExpression(components.initializer.expression)
        )
          for (const property of components.initializer.expression.properties)
            if (property.name)
              tags.add(
                property.name
                  .getText(sf)
                  .replaceAll('"', "")
                  .replaceAll("'", ""),
              );
        allowedTags.set(key.initializer.text, tags);
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(sf);
}
function signature(text, key) {
  const placeholders = [];
  let remaining = text.replace(
    /\{\{\s*([A-Za-z_][A-Za-z0-9_]*)(?:\s*,\s*([A-Za-z][A-Za-z0-9_]*))?\s*\}\}/g,
    (_all, name, format) => {
      placeholders.push(name + ":" + (format || ""));
      return "";
    },
  );
  if (remaining.includes("{{") || remaining.includes("}}"))
    errors.push("invalid placeholder " + key);
  const tags = [],
    stack = [],
    allowed = allowedTags.get(key) || new Set(["br"]);
  remaining = remaining.replace(
    /<(\/?)([A-Za-z][A-Za-z0-9_]*)(\/?)>/g,
    (_all, closing, name, self) => {
      if (!allowed.has(name)) errors.push("unsafe markup " + key + ":" + name);
      tags.push((closing ? "/" : "") + name + (self ? "/" : ""));
      if (name === "br" || self) return "";
      if (closing) {
        if (stack.pop() !== name) errors.push("unbalanced markup " + key);
      } else stack.push(name);
      return "";
    },
  );
  if (stack.length || /[<>]/.test(remaining))
    errors.push("invalid markup " + key);
  return { placeholders: placeholders.sort(), tags: tags.sort() };
}
let zh, en;
try {
  zh = catalog("zh-CN");
  en = catalog("en-US");
  const keys = new Set([...Object.keys(zh), ...Object.keys(en)]);
  for (const key of keys) {
    if (!(key in zh) || !(key in en)) {
      errors.push("missing key " + key);
      continue;
    }
    if (typeof zh[key] !== "string" || typeof en[key] !== "string") continue;
    const left = signature(zh[key], key),
      right = signature(en[key], key);
    if (
      JSON.stringify(left.placeholders) !== JSON.stringify(right.placeholders)
    )
      errors.push("placeholder mismatch " + key);
    if (JSON.stringify(left.tags) !== JSON.stringify(right.tags))
      errors.push("markup mismatch " + key);
    const plural = key.match(/^(.*)_(?:zero|one|two|few|many|other)$/);
    if (
      plural &&
      (!(plural[1] + "_one" in en) || !(plural[1] + "_other" in en))
    )
      errors.push("incomplete plural family " + plural[1]);
  }
} catch (error) {
  errors.push(error.message);
}
let legacy = [];
function covered(file, node) {
  if (
    file.startsWith("execution-pools/") ||
    file.startsWith("i18n/") ||
    [
      "ui.tsx",
      "main.tsx",
      "api.ts",
      "tool-hooks.ts",
      "RecordLineage.tsx",
      "record-options.ts",
    ].includes(file)
  )
    return true;
  if (file === "Governance.tsx") {
    for (let current = node; current; current = current.parent)
      if (
        ts.isFunctionDeclaration(current) &&
        current.name &&
        ["OrganizationSwitch", "OrganizationCreate"].includes(current.name.text)
      )
        return true;
    return false;
  }
  if (file !== "App.tsx") return false;
  for (let current = node; current; current = current.parent) {
    if (
      ts.isFunctionDeclaration(current) &&
      current.name &&
      [
        "Auth",
        "App",
        "ProjectCounts",
        "useOperationGuard",
        "Records",
        "RecordForm",
        "Detail",
        "CreateProject",
      ].includes(current.name.text)
    )
      return true;
    if (
      ts.isVariableDeclaration(current) &&
      ts.isIdentifier(current.name) &&
      ["nav", "tabs", "names", "priorityNames"].includes(current.name.text)
    )
      return true;
  }
  return false;
}
for (const file of sourceFiles) {
  const relative = path.relative(sourceRoot, file).replaceAll("\\", "/"),
    sf = ts.createSourceFile(
      file,
      fs.readFileSync(file, "utf8"),
      ts.ScriptTarget.Latest,
      true,
      file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
    ),
    found = new Map();
  function visit(node) {
    if (
      (ts.isStringLiteralLike(node) || ts.isJsxText(node)) &&
      /[\u3400-\u9fff]/.test(node.text)
    ) {
      if (relative === "i18n/LanguageSelector.tsx" && node.text === "简体中文")
        return;
      const hash = crypto.createHash("sha256").update(node.text).digest("hex");
      if (covered(relative, node))
        errors.push(
          "new hardcoded UI text " +
            relative +
            ":" +
            (sf.getLineAndCharacterOfPosition(node.getStart(sf)).line + 1),
        );
      else found.set(hash, (found.get(hash) || 0) + 1);
    }
    ts.forEachChild(node, visit);
  }
  visit(sf);
  if (found.size)
    legacy.push({
      file: relative,
      strings: [...found]
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([sha256, count]) => ({ sha256, count })),
      candidates: [...found.values()].reduce((a, b) => a + b, 0),
    });
}
legacy.sort((a, b) => a.file.localeCompare(b.file));
if (!args.includes("--skip-audit")) {
  if (args.includes("--update-baseline")) {
    fs.writeFileSync(
      baselinePath,
      JSON.stringify(
        {
          requirement: "FND-I18N-001",
          scope:
            "AST Han-bearing production literals and JSX text, excluding comments/tests/native language name; candidate counts are not complete translation-key estimates.",
          remaining: legacy,
        },
        null,
        2,
      ) + "\n",
    );
  } else {
    try {
      const baseline = JSON.parse(fs.readFileSync(baselinePath, "utf8"));
      for (const file of legacy) {
        const old = baseline.remaining.find((item) => item.file === file.file);
        for (const item of file.strings) {
          const previous = old?.strings.find(
            (entry) => entry.sha256 === item.sha256,
          );
          if (!previous || item.count > previous.count)
            errors.push(
              "legacy hardcode regression " +
                file.file +
                " " +
                item.sha256.slice(0, 12),
            );
        }
      }
    } catch {
      errors.push(
        "missing or invalid legacy baseline; review an explicit --update-baseline",
      );
    }
  }
}
if (errors.length) {
  process.stderr.write([...new Set(errors)].join("\n") + "\n");
  process.exitCode = 1;
} else {
  process.stdout.write(
    `Catalog keys ${Object.keys(zh).length}: language/placeholder/plural/markup checks passed.\n`,
  );
  if (!args.includes("--skip-audit"))
    process.stdout.write(
      `Legacy candidate strings ${legacy.reduce((sum, item) => sum + item.candidates, 0)} across ${legacy.length} modules remain explicitly unlocalized; covered-module hardcodes 0.\n`,
    );
}
