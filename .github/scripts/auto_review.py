#!/usr/bin/env python3
"""Auto-review changed Go and Python files using the Claude API.

Exit codes:
  0  — no critical issues found (or ANTHROPIC_API_KEY not set)
  1  — at least one file received a review containing a CRITICAL: bullet
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import urllib.request
from typing import NamedTuple

import anthropic

MODEL = "claude-sonnet-4-20250514"
MAX_FILE_BYTES = 20_000  # truncate very large files before sending

REVIEW_PROMPT = (
    "Review this code for: critical bugs, security issues, error handling gaps. "
    "Output only issues found, max 5 bullet points. "
    "Prefix each critical issue with 'CRITICAL:'. "
    "If no issues are found, output exactly 'LGTM'."
)


# ── data types ────────────────────────────────────────────────────────────────

class FileReview(NamedTuple):
    path: str
    review: str

    @property
    def is_lgtm(self) -> bool:
        return self.review.upper().startswith("LGTM")

    @property
    def has_critical(self) -> bool:
        return "CRITICAL:" in self.review


# ── git helpers ───────────────────────────────────────────────────────────────

def get_changed_files(base_sha: str, head_sha: str) -> list[str]:
    """Return .go and .py files added/modified/renamed between base and head."""
    result = subprocess.run(
        [
            "git", "diff", "--name-only", "--diff-filter=ACMR",
            f"{base_sha}...{head_sha}",
        ],
        capture_output=True,
        text=True,
        check=True,
    )
    return [
        f for f in result.stdout.splitlines()
        if f.endswith((".go", ".py")) and os.path.isfile(f)
    ]


# ── review logic ──────────────────────────────────────────────────────────────

def review_file(client: anthropic.Anthropic, path: str) -> str:
    try:
        raw = open(path, encoding="utf-8", errors="replace").read()
    except OSError as exc:
        return f"Could not read file: {exc}"

    if len(raw) > MAX_FILE_BYTES:
        raw = raw[:MAX_FILE_BYTES] + "\n[... file truncated for review ...]"

    response = client.messages.create(
        model=MODEL,
        max_tokens=512,
        messages=[
            {
                "role": "user",
                "content": f"{REVIEW_PROMPT}\n\n```\n# {path}\n{raw}\n```",
            }
        ],
    )
    return response.content[0].text.strip()


# ── GitHub comment ────────────────────────────────────────────────────────────

def post_pr_comment(token: str, repo: str, pr_number: str, body: str) -> None:
    url = f"https://api.github.com/repos/{repo}/issues/{pr_number}/comments"
    data = json.dumps({"body": body}).encode()
    req = urllib.request.Request(
        url,
        data=data,
        method="POST",
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/json",
            "X-GitHub-Api-Version": "2022-11-28",
        },
    )
    with urllib.request.urlopen(req, timeout=15) as resp:
        if resp.status not in (200, 201):
            raise RuntimeError(f"GitHub API returned HTTP {resp.status}")


def build_comment(reviews: list[FileReview], model: str) -> str:
    lines: list[str] = [
        "## Automated Code Review\n\n",
        f"Reviewed **{len(reviews)}** changed file(s) "
        f"with [`{model}`](https://anthropic.com).\n\n",
    ]
    for rev in reviews:
        if rev.has_critical:
            icon = "🔴"
        elif rev.is_lgtm:
            icon = "✅"
        else:
            icon = "⚠️"
        lines.append(f"### {icon} `{rev.path}`\n\n{rev.review}\n\n")

    critical_paths = [r.path for r in reviews if r.has_critical]
    if critical_paths:
        lines.append(
            "---\n> ⛔ **Job failed** — critical issues found in: "
            + ", ".join(f"`{p}`" for p in critical_paths)
            + "\n"
        )
    return "".join(lines)


# ── entry point ───────────────────────────────────────────────────────────────

def main() -> int:
    api_key = os.environ.get("ANTHROPIC_API_KEY", "")
    github_token = os.environ.get("GITHUB_TOKEN", "")
    pr_number = os.environ.get("PR_NUMBER", "")
    repo = os.environ.get("REPO", "")
    base_sha = os.environ.get("BASE_SHA", "")
    head_sha = os.environ.get("HEAD_SHA", "")

    if not api_key:
        print(
            "ANTHROPIC_API_KEY not set — skipping auto-review.\n"
            "Add it as a repository secret to enable automated code review.",
            file=sys.stderr,
        )
        return 0

    if not (base_sha and head_sha):
        print("BASE_SHA / HEAD_SHA not set — cannot determine changed files.", file=sys.stderr)
        return 1

    client = anthropic.Anthropic(api_key=api_key)

    changed = get_changed_files(base_sha, head_sha)
    if not changed:
        print("No .go or .py files changed — nothing to review.")
        return 0

    print(f"Auto-reviewing {len(changed)} file(s): {', '.join(changed)}")

    reviews: list[FileReview] = []
    for path in changed:
        print(f"  → {path} …", flush=True)
        text = review_file(client, path)
        rev = FileReview(path=path, review=text)
        reviews.append(rev)
        status = "CRITICAL" if rev.has_critical else ("LGTM" if rev.is_lgtm else "issues")
        print(f"     [{status}]")

    comment_body = build_comment(reviews, MODEL)

    if github_token and pr_number and repo:
        try:
            post_pr_comment(github_token, repo, pr_number, comment_body)
            print("PR comment posted.")
        except Exception as exc:
            print(f"Warning: could not post PR comment: {exc}", file=sys.stderr)
    else:
        print("\n--- Review output (no GitHub env vars set) ---")
        print(comment_body)

    has_critical = any(r.has_critical for r in reviews)
    if has_critical:
        critical_files = [r.path for r in reviews if r.has_critical]
        print(
            f"\n❌ CRITICAL issues found in: {', '.join(critical_files)}\n"
            "Resolve the issues above before merging.",
            file=sys.stderr,
        )
        return 1

    print("\n✅ No critical issues found.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
