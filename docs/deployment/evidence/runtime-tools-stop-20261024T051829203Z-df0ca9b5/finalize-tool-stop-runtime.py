"""Finalize a new immutable stop-fix runtime archive after root's full Go QA.

This only reads runtime/test receipts and adds a new document/evidence cut. It
does not rebuild, launch binaries, read credentials, or change prior evidence.
"""
from pathlib import Path
import argparse
import datetime
import hashlib
import json
import re
import shutil
import struct


parser = argparse.ArgumentParser()
parser.add_argument("--stamp", required=True)
parser.add_argument("--audit", required=True)
args = parser.parse_args()
root = Path.cwd().resolve()
stamp = args.stamp
audit = (root / args.audit).resolve()
archive = root / "docs/deployment/evidence" / ("runtime-tools-stop-" + stamp)
doc = root / "docs/deployment/tool-stop-runtime-bundles.md"
assert archive.is_dir() and audit.is_relative_to(root / ".cache") and not doc.exists()
assert not (archive / "final-verification.json").exists()
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
read = lambda p: json.loads(p.read_text(encoding="utf-8-sig"))
write = lambda p, obj: p.write_text(json.dumps(obj, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
verification = read(archive / "verification.json")
build = read(archive / "manifest.json")
sources = read(archive / "source-manifest.json")
assert len(sources) == 116 and len(verification["binaries"]) == 24 and len(build["commands"]) == 50
assert hashlib.sha256(json.dumps(sources, ensure_ascii=False, separators=(",", ":")).encode()).hexdigest() == verification["source_sha256"]
for x in sources:
    p = root / x["path"]
    assert p.stat().st_size == x["size_bytes"] and sha(p) == x["sha256"], x["path"]
for x in verification["binaries"]:
    p = root / x["path"]
    assert p.stat().st_size == x["size_bytes"] and sha(p) == x["sha256"], x["path"]
    b = p.read_bytes()[:4096]
    target, arch = x["target"], x["target"].split("-")[1]
    if target.startswith("windows"):
        off = struct.unpack_from("<I", b, 0x3C)[0]
        assert b[:2] == b"MZ" and b[off:off + 4] == b"PE\0\0"
        assert struct.unpack_from("<H", b, off + 4)[0] == {"amd64": 0x8664, "arm64": 0xAA64}[arch]
    elif target.startswith("linux"):
        assert b[:6] == b"\x7fELF\x02\x01" and struct.unpack_from("<H", b, 18)[0] == {"amd64": 62, "arm64": 183}[arch]
    else:
        assert b[:4] == bytes.fromhex("cffaedfe") and struct.unpack_from("<I", b, 4)[0] == {"amd64": 0x01000007, "arm64": 0x0100000C}[arch]
for name in ("manifest.json", "source-manifest.json", "linked-modules.json"):
    assert sha(archive / name) == sha(root / "dist/runtime/builds" / stamp / name)
for command in build["commands"]:
    assert command["exit_code"] == 0
    for key in ("stdout_log", "stderr_log"):
        assert sha(root / command[key]) == sha(archive / "logs" / Path(command[key]).name)
for row in read(archive / "sha256.json"):
    assert (archive / row["name"]).stat().st_size == row["size_bytes"] and sha(archive / row["name"]) == row["sha256"]

qa_path = root / ".cache/e2e/tool-stage/go-verification-post-review.json"
qa = read(qa_path)
assert qa["test_exit"] == qa["vet_exit"] == qa["failed_test_events"] == qa["skipped_test_cases"] == 0
assert qa["source_unchanged_since_start"] and qa["source_files"] == qa["source_after"]
qa_files = {x["path"].replace("\\", "/"): x["sha256"] for x in qa["source_files"]}
for x in sources:
    assert qa_files[x["path"]] == x["sha256"], "Runtime cut differs from full tested snapshot"
raw_path = qa_path.parent / qa["raw_file"]
assert sha(raw_path) == qa["raw_sha256"]
events = [json.loads(line) for line in raw_path.read_text(encoding="utf-8-sig").splitlines() if line]
passed = sum(x.get("Action") == "pass" and bool(x.get("Test")) for x in events)
failed = sum(x.get("Action") == "fail" for x in events)
skipped = sum(x.get("Action") == "skip" and bool(x.get("Test")) for x in events)
assert passed == qa["passed_test_events"] and failed == skipped == 0
shutil.copyfile(qa_path, archive / "go-test-receipt.original.json")
binding = {
    "schema_version": 1, "stage": "T-P4-004", "source_receipt": str(qa_path.relative_to(root)).replace("\\", "/"),
    "source_receipt_sha256": sha(qa_path), "archived_original_receipt": "go-test-receipt.original.json",
    "raw_source": str(raw_path.relative_to(root)).replace("\\", "/"), "raw_sha256": qa["raw_sha256"],
    "runtime_source_sha256": verification["source_sha256"], "runtime_files": len(sources),
    "test_snapshot_files": len(qa_files), "runtime_files_match_final_test_snapshot": True,
    "actual_pass_events": passed, "actual_failed_events": failed, "actual_skipped_test_cases": skipped,
    "test_exit": qa["test_exit"], "vet_exit": qa["vet_exit"], "started_at": qa["started_at"], "completed_at": qa["completed_at"],
    "scope": "Full root Go test/vet snapshot and actual raw JSONL digest rechecked, not rerun by this delivery agent. Quality archive retains the full raw log separately."
}
write(archive / "go-test-binding.json", binding)
regression = root / ".cache/e2e/tool-stage/stopped-zero-exit-verification.json"
reg = read(regression)
assert reg["targeted_test_exit"] == 0 and reg["records"][-1]["passed_test_events"] == 38
for source in [regression] + [regression.parent / r["file"] for r in reg["records"]]:
    dest = archive / source.name
    assert not dest.exists()
    shutil.copyfile(source, dest)
for row in reg["files_recorded_after_targeted_green"]:
    if row["path"].endswith(".go"):
        assert sha(root / row["path"]) == row["sha256"], row["path"]

history = read(archive / "historical-files.json")
protected, unchanged, changes = [], [], []
for x in history:
    p = root / x["path"]
    assert p.is_file(), x["path"]
    same = p.stat().st_size == x["size_bytes"] and sha(p) == x["sha256"]
    immutable = x["path"].startswith(("dist/runtime/", "docs/deployment/evidence/"))
    if immutable:
        assert same, x["path"]
        protected.append(x["path"])
    if same:
        unchanged.append(x["path"])
    else:
        assert x["path"].startswith("docs/deployment/") and x["path"].endswith(".md"), x["path"]
        changes.append({"path": x["path"], "previous_sha256": x["sha256"], "current_sha256": sha(p), "previous_size_bytes": x["size_bytes"], "current_size_bytes": p.stat().st_size})
assert sha(root / "docs/deployment/tool-runtime-bundles.md") == "149ddaa61aaa808b45b43d0c010cf3f93ac2ea9c1d9a4ecc71324e583e3e3615"
assert sha(root / "docs/quality/evidence/tool-go-tests.jsonl") == "db5a89e2792d2d4dec579ac30e208aee27d6e520c4323c6ca4f1de05b8cdf828"
prep = read(archive / "preparation.json")
for x in prep["existing_build_stamps"]:
    assert (root / "dist/runtime/builds" / x).is_dir()
now = datetime.datetime.now(datetime.timezone.utc).isoformat()
write(archive / "historical-preservation-final.json", {"schema_version": 1, "recorded_at": now, "prebuild_snapshot_files": len(history), "all_snapshot_files_unchanged": len(unchanged), "immutable_artifact_and_evidence_files_checked": len(protected), "immutable_files_unchanged": True, "current_document_updates": changes, "prior_tool_document_unchanged": True, "prior_479_raw_unchanged": True})

prefix = "evidence/runtime-tools-stop-" + stamp + "/"
target_rows = "\n".join(f"| {x['target']} | {x['format']} | 4 | {x['size_bytes']:,} | {x['linked_modules']} |" for x in verification["target_summary"])
win_rows = "\n".join(f"| {x['target']} | {x['name']} | {x['size_bytes']:,} | `{x['sha256']}` |" for x in verification["binaries"] if x["target"].startswith("windows"))
delta = read(archive / "cut-difference.json")
delta_rows = "、".join("`" + x["path"] + "`" for x in delta["changed"])
document = f"""# T-P4-004 停止语义修复后的正式运行包

日期：2026-10-24。交付批次：`{stamp}`。本批次重新构建六目标各四程序，共24个程序，并逐文件独立核对构建信息、大小、SHA-256与PE/ELF/Mach-O架构。

## 冻结范围与实际变化

运行输入仍为116份：114份非测试Go源码及go.mod、go.sum；清单摘要为 `{verification['source_sha256']}`。构建前后、独立验证及最终归档再次读取均一致。它是有序紧凑JSON清单的摘要，区别于格式化source-manifest.json的文件摘要。

相对[首轮工具执行包](tool-runtime-bundles.md)的 `20261024T045219750Z-715ba2c8`，新增{len(delta['added'])}个运行输入、修改{len(delta['changed'])}个、删除{len(delta['removed'])}个：{delta_rows}。见[逐项差异]({prefix}cut-difference.json)。停止回执在非取消运行中统一记录失败；即使OS退出0、工具证据缺失或协议声明完成，也不能启动依赖成功的下游。取消请求后的真实停止仍记录取消，外部效果未知仍进入核对。Codex用量接受官方最小三项input/cached/output；可选reasoning/cache-write字段如存在仍严格校验，不宣称本机0.164实际省略了这些字段。合同见[工具执行规范](../../contracts/tool-execution.md)。

网页、TS原生智能体、知识服务、Electron及外部CLI安装不在Go运行包中，各自验证范围由对应证据说明；本批次不替换历史桌面安装包的验收结论。

## 构建与独立核对

宿主Windows amd64、PowerShell7、Go `{build['toolchain']['version']}`。实际UTC构建区间 `{build['started_at']}` 至 `{build['completed_at']}`。调用原有 `scripts/build-runtime.ps1 -All`，子进程GOMAXPROCS=2，原脚本退出0。50次Go调用（工具链1、离线模块校验1、编译24、元数据读取24）全部成功；独立复核又读取24份实际文件元数据与二进制头。

| 目标 | 实际格式 | 程序数 | 总字节 | 第三方模块并集 |
|---|---|---:|---:|---:|
{target_rows}

实际元数据核对GOOS/GOARCH、CGO_ENABLED=0与trimpath；原始调用回执绑定buildvcs=false、mod=readonly及其余参数。依赖网络、工具链自动下载、VCS与telemetry关闭。缓存和临时目录位于仓库.cache；用户/机器环境只记录摘要并保持未变，见[环境核对]({prefix}environment-verification.json)。跨目标链接模块并集{verification['linked_module_count']}项，与前一工具运行包的标识、版本、校验和及replace逐项比较见[依赖核对]({prefix}linked-module-comparison.json)；这不构成完整供应链、许可或安全验收。

## 实际交付位置与Windows字节

运行文件位于 `D:\\codes\\ai-dev-os\\suma\\dist\\runtime\\<target>\\{stamp}`，四程序为suma-control、suma-runner、suma-deployer、suma（Windows有.exe扩展名）。原始构建收据位于 `dist/runtime/builds/{stamp}`。全部24份绝对路径、大小、摘要及格式见[独立结果]({prefix}verification.json)。

| Windows目标 | 程序 | 字节 | SHA-256 |
|---|---|---:|---|
{win_rows}

本交付代理不启动或停止任何程序；根代理后续激活应核对本表字节并单独记录真实运行证据，不能沿用首轮c06d/12c程序的运行结论。

## 测试切面、原件与历史保留

根代理修复后重新执行完整Go测试和vet：**{passed}个通过测试事件、0失败、0测试跳过**。共{len(qa_files)}份测试源码快照，运行包116个输入逐文件精确匹配；实际原始JSONL重新计算摘要为 `{qa['raw_sha256']}`。见[完整测试绑定]({prefix}go-test-binding.json)和[未改写的原始摘要副本]({prefix}go-test-receipt.original.json)。原始测试日志由质量档案单独保存，本交付步骤不重跑测试。

停止修复先保存实际RED，再实际GREEN：20格DAG停止/未知矩阵、4种工具证据的真实HTTP边界、固定候选测试停止、原取消/正常验收/补偿用例，目标共38个通过事件。最终RED有8个真实缺陷叶用例失败，见[修复验证]({prefix}stopped-zero-exit-verification.json)、[最终RED]({prefix}stopped-zero-exit-red-attempt3.jsonl)及[GREEN]({prefix}stopped-zero-exit-green.jsonl)。首个RED中的issue关联断言误查旧列，随后改为正式data.run_id，原失败尝试保留；成功测试夹具原误设Stopped的五处已改为正常完成事实，生产没有绕过规则。

三份构建清单从dist逐字节复制，50对原始stdout/stderr、24对独立metadata输出及原脚本/包装器/验证器均保存。二进制仅在忽略的dist中，未复制到文档目录。最初证据摘要见[初始清单]({prefix}sha256.json)；[最终复核]({prefix}final-verification.json)绑定本页、运行源码、程序字节、原件和新Go切面，[最终摘要清单]({prefix}sha256-final.json)覆盖新证据目录除自身以外的全部原字节。

预构建历史快照{len(history)}份，其中{len(protected)}份旧运行产物/部署证据严格保留；最终相同{len(unchanged)}份，当前指南更新逐项另列于[历史复核]({prefix}historical-preservation-final.json)。首轮工具文档、479测试原始日志和旧批次177文件仍保持原字节；旧结果是旧切面，不能描述为已经包含这次停止/用量修复。

## 验收范围

本页证明离线交叉编译、宿主读取的元数据/文件格式/大小/摘要、精确源码与新完整Go测试切面绑定及历史保留。目标系统执行、签名、安装升级、外部账号/模型、分布式集群、生产与规模指标由独立证据证明。macOS/Linux和Windows arm64没有由交付代理实际运行；TS/网页/桌面也需分别验证，不以24份程序推断完整平台P0–P7已交付。
"""
doc.write_text(document, encoding="utf-8", newline="\n")
assert not re.search(r"(?m)[ \t]+$", document)
links = [x.split("#", 1)[0] for x in re.findall(r"\]\(([^)]+)\)", document)]
for link in links:
    if Path(link).name not in {"final-verification.json", "sha256-final.json"}:
        assert (doc.parent / link).is_file(), link
shutil.copyfile(Path(__file__), archive / "finalize-tool-stop-runtime.py")
final = {
    "schema_version": 1, "recorded_at": now, "stage": "T-P4-004", "stamp": stamp,
    "source_sha256": verification["source_sha256"], "runtime_source_files_rechecked": len(sources),
    "current_runtime_sources_match": True, "artifacts_rechecked": len(verification["binaries"]),
    "independent_go_metadata_checks": 24, "original_receipt_copies_identical": 3,
    "original_stdout_stderr_pairs_identical": 50, "go_test_runtime_source_binding": True,
    "passed_test_events": passed, "failed_test_events": failed, "skipped_test_cases": skipped,
    "test_snapshot_files": len(qa_files), "documentation": str(doc.relative_to(root)).replace("\\", "/"),
    "documentation_sha256": sha(doc), "historical_files_rechecked": len(history),
    "historical_files_unchanged": len(unchanged), "immutable_historical_files_unchanged": len(protected),
    "current_document_updates": changes, "historical_build_receipt_directories": len(prep["existing_build_stamps"]),
    "prior_tool_document_unchanged": True, "prior_479_raw_unchanged": True,
    "windows_binaries": [x for x in verification["binaries"] if x["target"] == "windows-amd64"],
    "no_target_program_started_or_stopped_by_delivery_agent": True, "no_binary_copied_to_documentation": True,
    "scope": "Offline build/independent metadata/headers/bytes, new full Go snapshot binding and history preservation. Root activation/browser evidence, target systems, signing, TS and production are separate."
}
write(archive / "final-verification.json", final)
files = [{"name": p.relative_to(archive).as_posix(), "size_bytes": p.stat().st_size, "sha256": sha(p)} for p in sorted(archive.rglob("*")) if p.is_file() and p.name != "sha256-final.json"]
write(archive / "sha256-final.json", files)
for x in files:
    assert (archive / x["name"]).stat().st_size == x["size_bytes"] and sha(archive / x["name"]) == x["sha256"]
for link in links:
    assert (doc.parent / link).is_file()
assert not any(p.suffix.lower() in {".exe", ".dll", ".db", ".key", ".pem"} for p in archive.rglob("*") if p.is_file())
print(json.dumps({"status": "verified", "stamp": stamp, "source_sha256": verification["source_sha256"], "evidence_files": len(files) + 1, "passed_test_events": passed, "test_snapshot_files": len(qa_files), "documentation_sha256": sha(doc), "final_verification_sha256": sha(archive / "final-verification.json"), "final_manifest_sha256": sha(archive / "sha256-final.json"), "windows": final["windows_binaries"]}))
