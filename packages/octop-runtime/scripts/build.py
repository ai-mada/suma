"""Build a deterministic standard-library zipapp and its administrator pin manifest.

Third-party distributions remain in the protected managed Python environment;
the manifest records exact installed versions, not a claim of OS isolation.
"""

from __future__ import annotations

import hashlib
import importlib.metadata
import json
import pathlib
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCE = ROOT / "src"
DIST = ROOT / "dist"


def main():
    versions = {dist.metadata["Name"].lower().replace("_", "-"): dist.version for dist in importlib.metadata.distributions()}
    for name, required in {"octop-harness": "1.0.5", "agent-client-protocol": "0.12.1", "mcp": "1.30.0"}.items():
        if versions.get(name) != required:
            raise SystemExit("Build with the pinned managed environment: " + name)
    versions = dict(sorted(versions.items()))
    DIST.mkdir(exist_ok=True)
    target = DIST / "suma-octop.pyz"
    with zipfile.ZipFile(target, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for file in sorted(SOURCE.rglob("*.py")):
            info = zipfile.ZipInfo(file.relative_to(SOURCE).as_posix(), date_time=(2026, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o644 << 16
            archive.writestr(info, file.read_bytes())
        info = zipfile.ZipInfo("suma_octop/dependency-lock.json", date_time=(2026, 1, 1, 0, 0, 0))
        archive.writestr(info, json.dumps(versions, sort_keys=True, separators=(",", ":")))
    manifest = {"schema_version": 1, "entry": target.name, "entry_sha256": hashlib.sha256(target.read_bytes()).hexdigest(), "external_imports": [], "dependencies": versions}
    (DIST / "suma-octop.manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"entry": str(target), "manifest": str(DIST / "suma-octop.manifest.json"), "entry_sha256": manifest["entry_sha256"]}))


if __name__ == "__main__":
    main()
