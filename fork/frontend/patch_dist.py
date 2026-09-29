"""把前端补丁打进已构建的 dist，供镜像构建使用。

用途：镜像里直接带上三处补丁，部署后无需再往 data/public/ 放覆盖文件。
补丁逻辑与 旧补丁/前端补丁/build_patches.py 一致，只是输入从「临时文件/live-*.js」
换成「dist 里刚构建出来的 chunk」。

入口 chunk 从 dist/index.html 读取：assets/ 下还有别的 index-*.js 路由分包，
按文件名通配会命中多个，必须按 index.html 的引用定位。

每个模式必须命中且仅命中一次，否则直接失败退出。

用法：
    python fork/frontend/patch_dist.py --dist backend/internal/web/dist
    python fork/frontend/patch_dist.py --dist <dir> --dry-run   # 只报告不改文件
"""
import argparse
import hashlib
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
INJECT = os.path.join(HERE, "group-model-accounts-injection.js")

# 与 旧补丁/前端补丁/build_patches.py 保持一致
ENTRY_PATTERN = re.compile(
    r"async function (\w+)\(e\)\{const\{data:t\}=await n\.get\(`/admin/accounts/\$\{e\}/models`\);return t\}"
)
ENTRY_TEMPLATE = (
    "async function {fn}(e){"
    "try{"
    "const{data:r}=await n.get(`/admin/accounts/${e}`);"
    'if(r&&r.platform==="openai"){'
    "const m=(r.credentials&&r.credentials.model_mapping)||{};"
    "const s=new Set([...Object.keys(m),...Object.values(m)]);"
    'if(s.size){return[...s].map(k=>({id:k,display_name:k,object:"model",created:0,owned_by:"openai"}))}'
    "}"
    "}catch(r){}"
    "const{data:t}=await n.get(`/admin/accounts/${e}/models`);"
    "return t}"
)
CSV_PATTERN = re.compile(r"return;(\w+)\.value=(\w+)\.items\|\|\[\]\}")

PATCH_MARK = "group-model-accounts-panel"
SORT_MARK = ".slice().sort((A,B)=>A.id-B.id)"
ENTRY_RE = re.compile(r"assets/(index-[A-Za-z0-9_-]+\.js)")
CSV_RE = re.compile(r"ChannelStatusView-[A-Za-z0-9_-]+\.js")


def single_match(pattern, text, what):
    hits = sorted(set(pattern.findall(text)))
    if len(hits) != 1:
        sys.exit(f"[FAIL] {what} 命中 {len(hits)} 个（要求恰好 1 个）："
                 + (", ".join(hits) if hits else "(无)"))
    return hits[0]


def read(path):
    with open(path, "r", encoding="utf-8", newline="") as f:
        return f.read()


def write(path, text):
    with open(path, "w", encoding="utf-8", newline="") as f:
        f.write(text)


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", required=True, help="dist 目录（含 assets/ 与 index.html）")
    parser.add_argument("--dry-run", action="store_true", help="只检查模式命中，不写文件")
    args = parser.parse_args()

    assets = os.path.join(args.dist, "assets")
    if not os.path.isdir(assets):
        sys.exit(f"[FAIL] 找不到 {assets}")
    html_path = os.path.join(args.dist, "index.html")
    if not os.path.isfile(html_path):
        sys.exit(f"[FAIL] 找不到 {html_path}")

    html = read(html_path)
    entry_name = single_match(ENTRY_RE, html, "index.html 里的入口 chunk 引用")
    entry_path = os.path.join(assets, entry_name)
    if not os.path.isfile(entry_path):
        sys.exit(f"[FAIL] 找不到 {entry_path}")

    entry = read(entry_path)
    csv_name = single_match(CSV_RE, entry, "入口里的渠道监控 chunk 引用")
    csv_path = os.path.join(assets, csv_name)
    if not os.path.isfile(csv_path):
        sys.exit(f"[FAIL] 找不到 {csv_path}")
    print(f"入口 chunk: {entry_name}")
    print(f"监控 chunk: {csv_name}")

    if PATCH_MARK in entry:
        sys.exit("[FAIL] 入口已含补丁标记，dist 可能已被打过补丁，不要重复打")

    matches = ENTRY_PATTERN.findall(entry)
    if len(matches) != 1:
        sys.exit(f"[FAIL] 入口 getAvailableModels: 命中 {len(matches)} 次（要求恰好 1 次），"
                 f"压缩实现可能已变化，需人工确认定位方式")
    fn = matches[0]
    old_entry = ENTRY_PATTERN.search(entry).group(0)
    new_entry = ENTRY_TEMPLATE.replace("{fn}", fn)
    print(f"[OK] 入口 getAvailableModels（函数名 {fn}）: 命中 1 次，{len(old_entry)} -> {len(new_entry)} 字符")

    csv = read(csv_path)
    csv_matches = CSV_PATTERN.findall(csv)
    if len(csv_matches) != 1:
        sys.exit(f"[FAIL] 渠道监控排序: 命中 {len(csv_matches)} 次（要求恰好 1 次），"
                 f"压缩实现可能已变化，需人工确认定位方式")
    obj, items = csv_matches[0]
    old_csv = CSV_PATTERN.search(csv).group(0)
    new_csv = f"return;{obj}.value=({items}.items||[]).slice().sort((A,B)=>A.id-B.id)}}"
    csv = csv.replace(old_csv, new_csv)
    print(f"[OK] 渠道监控按 ID 排序（{obj}/{items}）: 命中 1 次")

    if args.dry_run:
        print("\n[dry-run] 模式全部命中，未写入任何文件。")
        return

    write(entry_path, entry.replace(old_entry, new_entry) + read(INJECT))
    write(csv_path, csv)

    print("\n=== 产物校验 ===")
    for p in (entry_path, csv_path):
        text = read(p)
        marks = []
        if PATCH_MARK in text:
            marks.append(PATCH_MARK)
        if SORT_MARK in text:
            marks.append("sort-by-id")
        print(f"  {os.path.basename(p)}  {os.path.getsize(p)} 字节  sha256 {sha256(p)}  标记 {marks}")


if __name__ == "__main__":
    main()
