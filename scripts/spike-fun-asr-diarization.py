#!/usr/bin/env python3
"""Manual spike for Aliyun DashScope non-realtime ASR diarization.

This script intentionally uses only Python's standard library so it can run in
local dev without adding repo dependencies. It submits a non-realtime ASR
transcription task, polls until completion, downloads the transcription JSON,
and prints a compact speaker/segment summary. It can compare models such as
fun-asr and paraformer-v2 by changing --model.

Usage examples:
  DASHSCOPE_API_KEY=sk-... scripts/spike-fun-asr-diarization.py \
    --file-url https://example.com/audio.mp3

  # Experimental: only works if DashScope accepts file:// for this endpoint.
  DASHSCOPE_API_KEY=sk-... scripts/spike-fun-asr-diarization.py \
    --local-file testdata/asr/mimo-tts/mp3/zh-four-speaker-standup.mp3

  # Experimental: test whether the non-realtime file_urls endpoint accepts a
  # Base64 data URL instead of a public URL.
  DASHSCOPE_API_KEY=sk-... scripts/spike-fun-asr-diarization.py \
    --local-data-url testdata/asr/mimo-tts/mp3/zh-four-speaker-standup.mp3
"""

from __future__ import annotations

import argparse
import base64
import json
import mimetypes
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

DEFAULT_BASE_URL = "https://dashscope.aliyuncs.com/api/v1"
DEFAULT_MODEL = "paraformer-v2"


def load_dotenv(path: Path) -> None:
    if not path.exists():
        return
    for raw_line in path.read_text(encoding="utf-8", errors="ignore").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        name, value = line.split("=", 1)
        os.environ.setdefault(name.strip(), value.strip().strip('"').strip("'"))


def require_api_key() -> str:
    for name in ("DASHSCOPE_API_KEY", "ALIYUN_API_KEY", "BAILIAN_API_KEY"):
        value = os.environ.get(name, "").strip()
        if value and value not in {"replace-me", "***"}:
            return value
    raise SystemExit("missing DASHSCOPE_API_KEY/ALIYUN_API_KEY/BAILIAN_API_KEY")


def request_json(method: str, url: str, api_key: str, payload: dict[str, Any] | None = None) -> dict[str, Any]:
    body = None if payload is None else json.dumps(payload, ensure_ascii=False).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=body,
        method=method,
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "X-DashScope-Async": "enable",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise SystemExit(f"HTTP {exc.code} {url}: {detail}") from exc


def download_json(url: str) -> dict[str, Any]:
    with urllib.request.urlopen(url, timeout=60) as response:
        return json.loads(response.read().decode("utf-8"))


def to_file_url(path: Path) -> str:
    return urllib.parse.urljoin("file:", urllib.request.pathname2url(str(path.resolve())))


def to_data_url(path: Path) -> str:
    media_type = mimetypes.guess_type(path.name)[0] or "application/octet-stream"
    encoded = base64.b64encode(path.read_bytes()).decode("ascii")
    return f"data:{media_type};base64,{encoded}"


def describe_source(audio_url: str) -> str:
    if audio_url.startswith("data:"):
        media_type = audio_url.split(";", 1)[0].removeprefix("data:")
        return f"data:{media_type};base64,<redacted length={len(audio_url)}>"
    return audio_url


def extract_task_id(response: dict[str, Any]) -> str:
    task_id = response.get("output", {}).get("task_id") or response.get("task_id")
    if not task_id:
        raise SystemExit("submit response did not contain output.task_id:\n" + json.dumps(response, ensure_ascii=False, indent=2))
    return str(task_id)


def find_transcription_url(response: dict[str, Any]) -> str:
    results = response.get("output", {}).get("results", [])
    if not results:
        raise SystemExit("task response did not contain output.results:\n" + json.dumps(response, ensure_ascii=False, indent=2))
    first = results[0]
    status = first.get("subtask_status")
    if status != "SUCCEEDED":
        raise SystemExit("Fun-ASR subtask failed:\n" + json.dumps(first, ensure_ascii=False, indent=2))
    url = first.get("transcription_url")
    if not url:
        raise SystemExit("Fun-ASR result missing transcription_url:\n" + json.dumps(first, ensure_ascii=False, indent=2))
    return str(url)


def segment_speaker(segment: dict[str, Any]) -> str:
    for key in ("speaker_id", "speaker", "spk", "speaker_label"):
        if key in segment and segment[key] not in (None, ""):
            return str(segment[key])
    return "unknown"


def summarize_result(result: dict[str, Any]) -> dict[str, Any]:
    transcripts = result.get("transcripts", [])
    segments: list[dict[str, Any]] = []
    for transcript in transcripts:
        for sentence in transcript.get("sentences", []) or []:
            segments.append(sentence)

    speaker_counts: dict[str, int] = {}
    for segment in segments:
        speaker = segment_speaker(segment)
        speaker_counts[speaker] = speaker_counts.get(speaker, 0) + 1

    sample_segments = []
    for segment in segments[:20]:
        sample_segments.append(
            {
                "speaker": segment_speaker(segment),
                "begin_time": segment.get("begin_time"),
                "end_time": segment.get("end_time"),
                "text": segment.get("text", ""),
            }
        )

    return {
        "file_url": result.get("file_url"),
        "text": "\n".join(str(t.get("text", "")) for t in transcripts if t.get("text")),
        "transcript_count": len(transcripts),
        "sentence_count": len(segments),
        "speaker_counts": speaker_counts,
        "sample_segments": sample_segments,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="Run a manual Fun-ASR diarization spike")
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--file-url", help="Publicly downloadable audio URL")
    source.add_argument("--local-file", type=Path, help="Local audio file, converted to file:// URL")
    source.add_argument("--local-data-url", type=Path, help="Local audio file, converted to Base64 data URL")
    parser.add_argument("--model", default=os.environ.get("DASHSCOPE_ASR_MODEL", DEFAULT_MODEL))
    parser.add_argument("--base-url", default=os.environ.get("DASHSCOPE_BASE_URL", DEFAULT_BASE_URL))
    parser.add_argument("--language-hints", default="zh,en")
    parser.add_argument("--channel-id", type=int, action="append", default=[0], help="Channel id to transcribe; repeat for multi-channel audio")
    parser.add_argument("--diarization", dest="diarization", action="store_true", default=True, help="Enable speaker diarization via diarization_enabled=true")
    parser.add_argument("--no-diarization", dest="diarization", action="store_false", help="Disable speaker diarization")
    parser.add_argument("--timestamp-alignment", dest="timestamp_alignment", action="store_true", default=None, help="Enable Paraformer timestamp_alignment_enabled=true")
    parser.add_argument("--enable-words", action="store_true", help="Request word-level timestamps where supported")
    parser.add_argument("--vocabulary-id", help="Optional custom hot-word vocabulary_id")
    parser.add_argument("--dry-run", action="store_true", help="Print request payload without calling DashScope")
    parser.add_argument("--poll-interval", type=float, default=3.0)
    parser.add_argument("--timeout", type=float, default=300.0)
    parser.add_argument("--output", type=Path, default=Path("var/fun-asr-spike/result.json"))
    args = parser.parse_args()

    load_dotenv(Path(".env"))
    api_key = require_api_key()

    if args.local_file:
        if not args.local_file.exists():
            raise SystemExit(f"local file not found: {args.local_file}")
        audio_url = to_file_url(args.local_file)
    elif args.local_data_url:
        if not args.local_data_url.exists():
            raise SystemExit(f"local file not found: {args.local_data_url}")
        audio_url = to_data_url(args.local_data_url)
    else:
        audio_url = args.file_url

    base_url = args.base_url.rstrip("/")
    parameters: dict[str, Any] = {
        "channel_id": args.channel_id,
        "language_hints": [item.strip() for item in args.language_hints.split(",") if item.strip()],
    }
    if args.diarization:
        parameters["diarization_enabled"] = True
    if args.enable_words:
        parameters["enable_words"] = True
    if args.vocabulary_id:
        parameters["vocabulary_id"] = args.vocabulary_id
    if args.timestamp_alignment is True or (args.timestamp_alignment is None and args.model.startswith("paraformer")):
        parameters["timestamp_alignment_enabled"] = True

    payload = {
        "model": args.model,
        "input": {"file_urls": [audio_url]},
        "parameters": parameters,
    }

    if args.dry_run:
        print(json.dumps(payload, ensure_ascii=False, indent=2))
        return 0

    print(f"submitting ASR task: model={args.model} source={describe_source(audio_url)}", file=sys.stderr)
    submit_response = request_json("POST", f"{base_url}/services/audio/asr/transcription", api_key, payload)
    task_id = extract_task_id(submit_response)
    print(f"task_id={task_id}", file=sys.stderr)

    deadline = time.monotonic() + args.timeout
    task_response: dict[str, Any] | None = None
    while time.monotonic() < deadline:
        task_response = request_json("GET", f"{base_url}/tasks/{task_id}", api_key)
        status = task_response.get("output", {}).get("task_status") or task_response.get("task_status")
        print(f"task_status={status}", file=sys.stderr)
        if status == "SUCCEEDED":
            break
        if status in {"FAILED", "CANCELED", "UNKNOWN"}:
            raise SystemExit("Fun-ASR task failed:\n" + json.dumps(task_response, ensure_ascii=False, indent=2))
        time.sleep(args.poll_interval)
    else:
        raise SystemExit(f"timed out waiting for task {task_id}")

    assert task_response is not None
    transcription_url = find_transcription_url(task_response)
    print(f"downloading transcription_url={transcription_url}", file=sys.stderr)
    result = download_json(transcription_url)

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")

    summary = summarize_result(result)
    print(json.dumps({"task_id": task_id, "output": str(args.output), "summary": summary}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
