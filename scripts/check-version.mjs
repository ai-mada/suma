// Product versions share one release baseline. Dependency, protocol, workflow
// and data revision numbers deliberately do not participate in this check.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const root = new URL('../', import.meta.url);
const read = path => readFileSync(new URL(path, root), 'utf8');
const expected = JSON.parse(read('package.json')).version;
const failures = [];
function verify(path, actual) {
  if (actual !== expected) failures.push(`${path}: expected ${expected}, received ${actual ?? 'missing'}`);
}

verify('initial product baseline', '1.0.0');
for (const path of [
  'apps/web/package.json', 'apps/desktop/package.json',
  'packages/contracts/package.json', 'packages/knowledge/package.json',
  'packages/agent-runtime/package.json',
]) verify(path, JSON.parse(read(path)).version);
verify('internal/buildinfo/version.go', /const Version = "([^"]+)"/.exec(read('internal/buildinfo/version.go'))?.[1]);
const chart = read('deploy/helm/suma/Chart.yaml');
verify('Helm chart version', /^version:\s*"?([^"\s]+)"?\s*$/m.exec(chart)?.[1]);
verify('Helm appVersion', /^appVersion:\s*"?([^"\s]+)"?\s*$/m.exec(chart)?.[1]);
verify('Helm image tag', /^\s+tag:\s*"?([^"\s]+)"?\s*$/m.exec(read('deploy/helm/suma/values.yaml'))?.[1]);
verify('desktop preload identity', /version:\s*['"]([^'"]+)['"]/.exec(read('apps/desktop/src/preload.cjs'))?.[1]);
if (failures.length) {
  throw new Error(`Product version drift in ${fileURLToPath(root)}:\n${failures.join('\n')}`);
}
console.log(`Product ${expected}: workspace packages, Go identity, desktop and Helm agree.`);
