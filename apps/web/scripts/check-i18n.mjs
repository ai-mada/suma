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
const sourceRoot = path.resolve(option('--source-dir')||path.join(base, "src")),
  catalogDir = path.resolve(
    option("--catalog-dir") || path.join(sourceRoot, "i18n/locales"),
  );
const errors = [];
function catalog(locale) {
  const domains=path.join(catalogDir,'domains'),domainFiles=fs.existsSync(domains)?fs.readdirSync(domains).filter(name=>name.endsWith('.json')).sort():[];
  for(const name of domainFiles){
    const parsed=name.match(/^([a-z][a-z0-9-]*)\.(zh-CN|en-US)\.json$/);
    if(!parsed)throw Error('invalid domain catalog '+name);
    const other=parsed[2]==='zh-CN'?'en-US':'zh-CN';
    if(!domainFiles.includes(parsed[1]+'.'+other+'.json'))throw Error('missing domain catalog pair '+name);
  }
  const inputs=[path.join(catalogDir,locale+'.json'),...domainFiles.filter(name=>name.endsWith('.'+locale+'.json')).map(name=>path.join(domains,name))],seen=new Set(),result=Object.create(null);
  for(const file of inputs){
    const raw=fs.readFileSync(file,'utf8');
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
  if(path.dirname(file)===domains){
    const prefix=path.basename(file).split('.')[0]+'.';
    for(const key of Object.keys(value))if(!key.startsWith(prefix))errors.push('domain namespace mismatch '+key);
  }
  Object.assign(result,value);
  }
  return result;
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
// Every product-owned production module now participates. The three exact
// immutable server filter identities and native language name are checked below.
for (const file of sourceFiles) {
  const relative = path.relative(sourceRoot, file).replaceAll("\\", "/"),
    sf = ts.createSourceFile(
      file,
      fs.readFileSync(file, "utf8"),
      ts.ScriptTarget.Latest,
      true,
      file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
    );
  function visit(node) {
    if (
      (ts.isStringLiteralLike(node) || ts.isJsxText(node) || ts.isTemplateHead(node) || ts.isTemplateMiddle(node) || ts.isTemplateTail(node)) &&
      /[\u3400-\u9fff]/.test(node.text)
    ) {
      if (relative === "i18n/LanguageSelector.tsx" && node.text === "简体中文")
        return;
      const hash = crypto.createHash("sha256").update(node.text).digest("hex");
      // Three immutable server policy-note identities are used only to filter
      // duplicate snapshot notes. Changing display language must not alter them.
      // No other source literal may inherit this narrowly reviewed exclusion.
      if (relative === 'SchedulingFacts.tsx' && [
        '1aa3c519a7d9b95a71f974980f4c617507d18f6ec0500fd23be3ff8bda1e84dc',
        '1aa27277262e2e7bb5c3b522e7d466c10d1e49869a6e3030a4f20ef19333d285',
        '5577ef68d320c1092d7fc163bffede3598eda654e7dfaff5c50538aa719d1ea7',
      ].includes(hash)) {
        for(let current=node;current;current=current.parent)
          if(ts.isVariableDeclaration(current)&&ts.isIdentifier(current.name)&&current.name.text==='standardPolicyNotes')return;
      }
      errors.push(
        "new hardcoded UI text " +
          relative +
          ":" +
          (sf.getLineAndCharacterOfPosition(node.getStart(sf)).line + 1),
      );
    }
    ts.forEachChild(node, visit);
  }
  visit(sf);
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
      "Production source hardcodes 0.\n",
    );
}
