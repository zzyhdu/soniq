#!/usr/bin/env python3
"""Review ASR transcript quality against a committed fixture text.

The script reads an expected fixture transcript from `testdata/asr/mimo-tts`,
loads a Soniq transcript from Postgres, and prints a lightweight quality report.
It does not read or print API keys.
"""

from __future__ import annotations

import argparse
import difflib
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_FIXTURE_DIR = ROOT / "testdata" / "asr" / "mimo-tts"
DEFAULT_COMPOSE_FILE = ROOT / "compose.temporal.yml"
DEFAULT_TERMS = ["Soniq", "MiMo", "Postgres", "Temporal", "mimo-v2.5-asr"]


def normalize_text(text: str) -> str:
    return re.sub(r"\s+", "", text).strip().lower()


def char_similarity(expected: str, actual: str) -> float:
    expected_norm = normalize_text(expected)
    actual_norm = normalize_text(actual)
    if not expected_norm and not actual_norm:
        return 1.0
    if not expected_norm or not actual_norm:
        return 0.0
    return difflib.SequenceMatcher(a=expected_norm, b=actual_norm).ratio()


def query_transcript(compose_file: Path, recording_id: str | None, title: str | None) -> dict[str, str]:
    if recording_id:
        escaped_id = recording_id.replace("'", "''")
        where = f"r.id = '{escaped_id}'"
    elif title:
        escaped_title = title.replace("'", "''")
        where = f"r.title = '{escaped_title}'"
    else:
        where = "rt.provider = 'openai_compatible_asr'"

    sql = f"""
SELECT json_build_object(
  'recording_id', r.id,
  'title', r.title,
  'recording_language', r.language,
  'status', r.status,
  'provider', rt.provider,
  'model', rt.model,
  'transcript_language', rt.language,
  'text', rt.text,
  'created_at', rt.created_at
)::text
FROM recording_transcripts rt
JOIN recordings r ON r.id = rt.recording_id
WHERE {where}
ORDER BY rt.created_at DESC
LIMIT 1;
"""
    command = [
        "docker",
        "compose",
        "-f",
        str(compose_file),
        "exec",
        "-T",
        "soniq-postgresql",
        "psql",
        "-U",
        "soniq_user",
        "-d",
        "soniq",
        "-P",
        "pager=off",
        "-A",
        "-t",
        "-c",
        sql,
    ]
    result = subprocess.run(command, check=True, text=True, capture_output=True)
    payload = result.stdout.strip()
    if not payload:
        raise SystemExit("no transcript row matched the requested recording/title")
    return json.loads(payload)


def term_report(expected: str, actual: str, terms: list[str]) -> list[dict[str, object]]:
    actual_lower = actual.lower()
    expected_lower = expected.lower()
    report = []
    for term in terms:
        expected_present = term.lower() in expected_lower
        actual_present = term.lower() in actual_lower
        if expected_present:
            report.append({"term": term, "present": actual_present})
    return report


def print_diff(expected: str, actual: str, max_lines: int) -> None:
    expected_chars = list(normalize_text(expected))
    actual_chars = list(normalize_text(actual))
    diff = list(difflib.unified_diff(expected_chars, actual_chars, fromfile="expected", tofile="actual", lineterm=""))
    if not diff:
        print("diff: exact match after whitespace/case normalization")
        return
    print(f"diff: showing first {max_lines} lines of char-level diff")
    for line in diff[:max_lines]:
        print(line)


def main() -> int:
    parser = argparse.ArgumentParser(description="Review ASR transcript against fixture source text")
    parser.add_argument("--fixture", required=True, help="Fixture stem, e.g. zh-product-terms")
    parser.add_argument("--recording-id", help="Recording ID to review")
    parser.add_argument("--title", help="Recording title to find; newest match wins")
    parser.add_argument("--fixture-dir", type=Path, default=DEFAULT_FIXTURE_DIR)
    parser.add_argument("--compose-file", type=Path, default=DEFAULT_COMPOSE_FILE)
    parser.add_argument("--terms", default=",".join(DEFAULT_TERMS), help="Comma-separated terms to check")
    parser.add_argument("--diff-lines", type=int, default=80)
    args = parser.parse_args()

    expected_path = args.fixture_dir / f"{args.fixture}.txt"
    if not expected_path.exists():
        raise SystemExit(f"fixture text not found: {expected_path}")

    expected = expected_path.read_text(encoding="utf-8")
    row = query_transcript(args.compose_file, args.recording_id, args.title)
    actual = row["text"] or ""
    score = char_similarity(expected, actual)
    terms = [term.strip() for term in args.terms.split(",") if term.strip()]

    print("ASR fixture transcript review")
    print("=============================")
    print(f"fixture: {expected_path.relative_to(ROOT)}")
    print(f"recording_id: {row['recording_id']}")
    print(f"title: {row['title']}")
    print(f"status: {row['status']}")
    print(f"provider: {row['provider']}")
    print(f"model: {row['model']}")
    print(f"recording_language: {row['recording_language']}")
    print(f"transcript_language: {row['transcript_language']}")
    print(f"char_similarity: {score:.3f}")
    print()
    print("expected:")
    print(expected.strip())
    print()
    print("actual:")
    print(actual.strip())
    print()
    reports = term_report(expected, actual, terms)
    if reports:
        print("terms:")
        for item in reports:
            status = "ok" if item["present"] else "missing"
            print(f"- {item['term']}: {status}")
        print()
    print_diff(expected, actual, args.diff_lines)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
