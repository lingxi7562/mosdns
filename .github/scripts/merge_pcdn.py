#!/usr/bin/env python3
"""
PCDN 规则多源合并脚本
拉取多个 PCDN 屏蔽源 → 解析为 AdGuard 格式 → 去重排序 → 输出 pcdn/pcdn2.txt
由 GitHub Actions 定时触发（.github/workflows/pcdn-update.yml）
"""
import urllib.request
import re
import sys
import os
import datetime

SOURCES = [
    # (名称, 候选 URL 列表)
    ("susetao/PCDNFilter-CHN-", [
        "https://raw.githubusercontent.com/susetao/PCDNFilter-CHN-/main/PCDNFilter.txt",
        "https://cdn.jsdelivr.net/gh/susetao/PCDNFilter-CHN-@main/PCDNFilter.txt",
    ]),
    ("4fuu/AdGuard-Home-PCDN", [
        "https://raw.githubusercontent.com/4fuu/AdGuard-Home-PCDN/refs/heads/main/ban.txt",
        "https://cdn.jsdelivr.net/gh/4fuu/AdGuard-Home-PCDN@main/ban.txt",
    ]),
    ("anti-AD discretion/pcdn.txt", [
        "https://raw.githubusercontent.com/privacy-protection-tools/anti-AD/master/discretion/pcdn.txt",
        "https://cdn.jsdelivr.net/gh/privacy-protection-tools/anti-AD@master/discretion/pcdn.txt",
    ]),
    ("thhbdd/Block-pcdn-domains", [
        "https://raw.githubusercontent.com/thhbdd/Block-pcdn-domains/main/ban.txt",
        "https://raw.githubusercontent.com/thhbdd/Block-pcdn-domains/master/ban.txt",
        "https://cdn.jsdelivr.net/gh/thhbdd/Block-pcdn-domains/ban.txt",
    ]),
]

TIMEOUT = 30
HEADERS = {"User-Agent": "pcdn-merge-bot/1.0"}


def fetch(url):
    """下载单个 URL，返回文本或 None"""
    req = urllib.request.Request(url, headers=HEADERS)
    with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
        return resp.read().decode("utf-8", errors="replace")


def fetch_source(name, urls):
    """尝试多个候选 URL，返回文本或 None"""
    for u in urls:
        try:
            data = fetch(u)
            print(f"  [ok] {name}: {u} ({len(data.splitlines())} lines)")
            return data
        except Exception as e:
            print(f"  [x ] {name}: {u} -> {e}")
    print(f"  [!!] {name}: ALL URLs FAILED")
    return None


def parse_rules(text):
    """解析规则文本，返回 (域名集合, 正则列表)"""
    domains = set()
    regexes = set()
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith(("#", "!", "[")):
            continue
        # AdGuard 正则: /regex/
        if line.startswith("/") and line.endswith("/"):
            regexes.add(line)
            continue
        # AdGuard 域名规则: ||domain^ 或 ||domain
        if line.startswith("||"):
            dom = line[2:].rstrip("^")
            if dom and re.match(r"^[a-z0-9.*_-]+(\.[a-z0-9-]+)+$", dom):
                domains.add(dom)
            continue
        # 纯域名行
        if re.match(r"^[a-z0-9.*_-]+(\.[a-z0-9-]+)+$", line):
            domains.add(line)
            continue
        # 其他格式（hosts 等）跳过
    return domains, regexes


def main():
    out_path = sys.argv[1] if len(sys.argv) > 1 else "pcdn/pcdn2.txt"
    all_domains = set()
    all_regexes = set()
    fetched_any = False

    for name, urls in SOURCES:
        text = fetch_source(name, urls)
        if text is None:
            continue
        fetched_any = True
        d, r = parse_rules(text)
        all_domains |= d
        all_regexes |= r
        print(f"  [{name}] +{len(d)} domains, +{len(r)} regexes")

    if not fetched_any:
        print("ERROR: no source fetched, aborting (keep existing file)")
        sys.exit(1)

    header = [
        "# PCDN merged rules (auto-generated)",
        f"# Generated: {datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d %H:%M UTC')}",
        f"# Sources: {', '.join(s[0] for s in SOURCES)}",
        f"# Domains: {len(all_domains)}, Regexes: {len(all_regexes)}",
        "# Format: AdGuard Home (mosdns adguard_rule)",
    ]
    lines = header + [f"||{d}^" for d in sorted(all_domains)] + sorted(all_regexes) + [""]

    os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
    with open(out_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    print(f"WROTE {out_path}: {len(all_domains)} domains + {len(all_regexes)} regexes")


if __name__ == "__main__":
    main()
