"""检查发布公告的长度、草稿处理和 HTTP 发送约定。"""

import io
import json
import unittest
from unittest.mock import patch

from notify_discord_release import build_payload, send_payload


class ReleaseNotificationTest(unittest.TestCase):
    def setUp(self):
        self.release = {
            "tagName": "v1.2.3",
            "name": "TokenRouter 1.2.3",
            "url": "https://github.com/TokenFlux/TokenRouter/releases/tag/v1.2.3",
            "body": "Fixed routing.\n@everyone",
            "isDraft": False,
            "isPrerelease": False,
        }
        self.run_url = "https://github.com/TokenFlux/TokenRouter/actions/runs/123"
        self.webhook = "https://discord.com/api/webhooks/123/test-token"

    def test_draft_is_rejected(self):
        self.release["isDraft"] = True
        with self.assertRaises(ValueError):
            build_payload(self.release, self.run_url)

    def test_notes_and_mentions(self):
        payload = build_payload(self.release, self.run_url)
        self.assertEqual(payload["allowed_mentions"], {"parse": []})
        self.assertEqual(payload["embeds"][0]["description"], self.release["body"])
        self.assertEqual(payload["embeds"][0]["url"], self.release["url"])

    def test_long_unicode_notes_fit_discord(self):
        self.release.update(name="📦" * 500, body="📦" * 6000)
        embed = build_payload(self.release, self.run_url)["embeds"][0]
        self.assertLessEqual(len(embed["title"].encode("utf-16-le")) // 2, 256)
        self.assertLessEqual(len(embed["description"].encode("utf-16-le")) // 2, 4096)

    def test_empty_notes_and_prerelease(self):
        self.release.update(name="", body="", isPrerelease=True)
        embed = build_payload(self.release, self.run_url)["embeds"][0]
        self.assertEqual(embed["title"], "TokenRouter v1.2.3")
        self.assertIn("GitHub", embed["description"])
        self.assertEqual(embed["fields"][1]["value"], "Pre-release")

    @patch("notify_discord_release.urllib.request.urlopen")
    def test_send_waits_for_receipt(self, open_url):
        open_url.return_value = io.BytesIO(b'{"id":"123"}')
        payload = build_payload(self.release, self.run_url)
        send_payload(self.webhook, payload)
        request = open_url.call_args.args[0]
        self.assertEqual(request.full_url, self.webhook + "?wait=true")
        self.assertEqual(request.method, "POST")
        self.assertEqual(json.loads(request.data), payload)
        self.assertEqual(open_url.call_args.kwargs["timeout"], 30)

    @patch("notify_discord_release.urllib.request.urlopen")
    def test_rejects_unexpected_destination(self, open_url):
        for url in [self.webhook + "/github", "https://example.com/hook", "http://discord.com/api/webhooks/123/token"]:
            with self.assertRaises(ValueError):
                send_payload(url, {})
        open_url.assert_not_called()

    @patch("notify_discord_release.urllib.request.urlopen")
    def test_missing_receipt_fails(self, open_url):
        open_url.return_value = io.BytesIO(b'{}')
        with self.assertRaises(ValueError):
            send_payload(self.webhook, {})


if __name__ == "__main__":
    unittest.main()
