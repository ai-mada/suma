import { afterEach, describe, it, expect } from "vitest";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { resolve, dirname, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
const repository = resolve(
    dirname(fileURLToPath(import.meta.url)),
    "../../../..",
  ),
  base = resolve(repository, ".cache/e2e/localization-stage/checker-fixtures"),
  script = resolve(repository, "apps/web/scripts/check-i18n.mjs");
const created: string[] = [];
function check(zh: string, en: string) {
  mkdirSync(base, { recursive: true });
  const folder = mkdtempSync(resolve(base, "case-"));
  created.push(folder);
  writeFileSync(resolve(folder, "zh-CN.json"), zh);
  writeFileSync(resolve(folder, "en-US.json"), en);
  return spawnSync(
    process.execPath,
    [script, "--catalog-dir", folder, "--skip-audit"],
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
