#!/usr/bin/env python3
"""在发布完成后，将 GitHub Release 摘要发送到 Discord。"""

import json
import os
import re
import sys
import urllib.error
import urllib.request


def build_payload(release, run_url):
    """生成公告内容，并限制 Discord embed 的文本长度。"""
    if release.get("isDraft"):
        raise ValueError("Draft releases cannot be announced.")

    tag = release["tagName"]
    title = release.get("name") or f"TokenRouter {tag}"
    notes = release.get("body") or "Release notes and downloads are available on GitHub."
    # UTF-16 长度也满足 Discord 的限制，emoji 会占用两个代码单元。
    title = title[:125]
    if len(notes) > 1900:
        notes = notes[:1900] + "\n\nRead the full release notes on GitHub."

    return {
        "username": "TokenRouter Releases",
        "allowed_mentions": {"parse": []},
        "embeds": [{
            "title": title,
            "url": release["url"],
            "description": notes,
            "color": 0x57F287,
            "fields": [
                {"name": "Version", "value": tag[:128], "inline": True},
                {
                    "name": "Release type",
                    "value": "Pre-release" if release.get("isPrerelease") else "Stable",
                    "inline": True,
                },
                {"name": "CI", "value": f"[Successful release workflow]({run_url})"},
            ],
        }],
    }


def send_payload(webhook_url, payload):
    """向 Discord 专用地址发消息，并等待服务端确认。"""
    if not re.fullmatch(r"https://discord\.com/api(?:/v\d+)?/webhooks/\d+/[A-Za-z0-9_-]+", webhook_url):
        raise ValueError("Expected a Discord webhook URL without a /github suffix.")

    request = urllib.request.Request(
        webhook_url + "?wait=true",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json", "User-Agent": "TokenRouter-Release-CI"},
        method="POST",
    )
    # 网络超时可能发生在消息送达之后，交给维护者检查后重试。
    with urllib.request.urlopen(request, timeout=30) as response:
        receipt = json.load(response)
    if not receipt.get("id"):
        raise ValueError("Discord did not return a message receipt.")


def main():
    """读取发布数据和 Actions Secret，发送失败时输出经过筛选的诊断。"""
    try:
        with open(sys.argv[1], encoding="utf-8") as source:
            release = json.load(source)
        payload = build_payload(release, os.environ["RELEASE_RUN_URL"])
        send_payload(os.environ["DISCORD_RELEASE_WEBHOOK_URL"], payload)
    except urllib.error.HTTPError as error:
        print(f"::warning::Discord notification failed (HTTP {error.code}).", file=sys.stderr)
        return 1
    except (OSError, ValueError, KeyError, IndexError):
        # 异常文本可能包含 Webhook 凭据，日志使用固定消息。
        print("::warning::Discord notification failed; check release data, webhook configuration, and connectivity.", file=sys.stderr)
        return 1
    print("Discord release announcement delivered.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
