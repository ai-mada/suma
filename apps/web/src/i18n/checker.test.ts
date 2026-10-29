import { afterEach, describe, it, expect } from "vitest";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { resolve, dirname, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
const repository = resolve(
    dirname(fileURLToPath(import.meta.url)),
    "../../../..",
  ),
  base = resolve(repository, ".cache/e2e/baseline-100-web/checker-fixtures"),
  script = resolve(repository, "apps/web/scripts/check-i18n.mjs");
const created: string[] = [];
function check(zh: string, en: string, domains: Record<string,string> = {}, sources?:Record<string,string>) {
  mkdirSync(base, { recursive: true });
  const folder = mkdtempSync(resolve(base, "case-"));
  created.push(folder);
  writeFileSync(resolve(folder, "zh-CN.json"), zh);
  writeFileSync(resolve(folder, "en-US.json"), en);
  if (Object.keys(domains).length) {
    mkdirSync(resolve(folder, "domains"));
    for (const [name, value] of Object.entries(domains))
      writeFileSync(resolve(folder, "domains", name), value);
  }
  const auditArguments:string[]=[];
  if(sources){
    const source=resolve(folder,'src');mkdirSync(source);
    for(const [file,value]of Object.entries(sources)){const target=resolve(source,file);mkdirSync(dirname(target),{recursive:true});writeFileSync(target,value);}
    auditArguments.push('--source-dir',source);
  }else auditArguments.push('--skip-audit');
  return spawnSync(
    process.execPath,
    [script, "--catalog-dir", folder, ...auditArguments],
    { encoding: "utf8" },
  );
}
afterEach(() => {
  for (const folder of created.splice(0)) {
    const inside = relative(base, resolve(folder));
    if (
      !inside ||
      inside.startsWith("..") ||
      inside.includes("/") ||
      inside.includes("\\") ||
      !inside.startsWith("case-")
    )
      throw Error("Refusing cleanup outside exact test-owned fixture");
    rmSync(folder, { recursive: true, force: true });
  }
});
describe("internationalization CI catalog contract", () => {
  // Each audit starts a separate Node process; keep its existing timeout per
  // independent source case instead of accumulating four startups in one test.
  it.each(['export const text="未翻译";','export const text=`原始错误 ${id}`;','export const text=`${id}错误${value}状态`;','export const text=<span>新增硬编码</span>;'])('rejects interpolated template fragments and hardcoded copy in any newly added production module (case %#)',(source)=>{
    const result=check('{}','{}',{}, {'future/Feature.tsx':source});expect(result.status,result.stdout).toBe(1);expect(result.stderr).toContain('new hardcoded UI text future/Feature.tsx');
  });
  it('keeps the native language-name exception specific to its selector',()=>{
    expect(check('{}','{}',{}, {'i18n/LanguageSelector.tsx':'export const native="简体中文";'}).status).toBe(0);
    const other=check('{}','{}',{}, {'future/Feature.tsx':'export const native="简体中文";'});expect(other.status).toBe(1);
  });
  it('excludes immutable policy filter identities only in their reviewed owner variable',()=>{
    const source='export const standardPolicyNotes=new Set(["容量为项目执行准入额度，并非实测硬件或操作系统硬限制"]);';
    expect(check('{}','{}',{}, {'SchedulingFacts.tsx':source}).status).toBe(0);
    const other=check('{}','{}',{}, {'SchedulingFacts.tsx':source.replace('standardPolicyNotes','arbitraryNotes')});expect(other.status).toBe(1);expect(other.stderr).toContain('new hardcoded UI text SchedulingFacts.tsx');
  });
  it("checks domain languages and does not silently ignore an unpaired domain", () => {
    const result=check('{}','{}',{'sample.zh-CN.json':'{"sample.title":"示例"}'});
    expect(result.status).toBe(1);
    expect(result.stderr).toContain('domain');
  });
  it("rejects a domain overriding a foundation key and mismatched domain parameters", () => {
    const duplicate=check('{"sample.title":"旧"}','{"sample.title":"Old"}',{
      'sample.zh-CN.json':'{"sample.title":"新"}',
      'sample.en-US.json':'{"sample.title":"New"}',
    });
    expect(duplicate.status).toBe(1);
    expect(duplicate.stderr).toContain('duplicate');
    const mismatch=check('{}','{}',{
      'sample.zh-CN.json':'{"sample.title":"{{name}}"}',
      'sample.en-US.json':'{"sample.title":"{{other}}"}',
    });
    expect(mismatch.status).toBe(1);
    expect(mismatch.stderr).toContain('placeholder');
  });
  it("accepts independent domain catalogs without changing foundation coverage", () => {
    const result=check('{"base":"基础"}','{"base":"Base"}',{
      'sample.zh-CN.json':'{"sample.title":"{{name}}的示例"}',
      'sample.en-US.json':'{"sample.title":"Example for {{name}}"}',
    });
    expect(result.status,result.stderr).toBe(0);
    expect(result.stdout).toContain('Catalog keys 2:');
  });
  it("rejects missing language keys instead of silently falling back", () => {
    const result = check('{"sample":"示例"}', "{}");
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("key");
  });
  it("rejects duplicate JSON keys and changed interpolation names", () => {
    const duplicate = check(
      '{"sample":"一","sample":"二"}',
      '{"sample":"Example"}',
    );
    expect(duplicate.status).toBe(1);
    const mismatch = check(
      '{"sample":"{{count}}项"}',
      '{"sample":"{{total}} items"}',
    );
    expect(mismatch.status).toBe(1);
    expect(mismatch.stderr).toContain("placeholder");
  });
  it("rejects unsafe markup and incomplete plural families", () => {
    expect(
      check('{"sample":"示例"}', '{"sample":"<script>bad</script>"}').status,
    ).toBe(1);
    const plural = check('{"sample_one":"一项"}', '{"sample_one":"One item"}');
    expect(plural.status).toBe(1);
    expect(plural.stderr).toContain("plural");
  });
  it("accepts matched placeholders, explicit zero-aware plural families and named text", () => {
    const result = check(
      '{"sample_one":"{{count}}项","sample_other":"{{count}}项"}',
      '{"sample_one":"{{count}} item","sample_other":"{{count}} items"}',
    );
    expect(result.status, result.stderr).toBe(0);
  });
});
