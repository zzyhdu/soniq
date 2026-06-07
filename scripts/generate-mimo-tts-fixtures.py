#!/usr/bin/env python3
"""Generate synthetic ASR audio fixtures with Xiaomi MiMo TTS.

The script reads source `.txt` files from `testdata/asr/mimo-tts`, calls
MiMo TTS (`mimo-v2-tts`) for each source text, and writes multiple input
audio formats for upload/normalization smoke tests.

It reads credentials from local environment or `.env` and never prints the
API key.
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_FIXTURE_DIR = ROOT / "testdata" / "asr" / "mimo-tts"
DEFAULT_MODEL = "mimo-v2-tts"
DEFAULT_VOICE = "default_zh"
DEFAULT_BASE_URL = "https://api.xiaomimimo.com/v1"

FORMATS = [
    {
        "directory": "wav-16k",
        "extension": "wav",
        "content_type": "audio/wav",
        "description": "WAV PCM 16 kHz mono",
        "ffmpeg_args": ["-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le"],
    },
    {
        "directory": "wav-24k",
        "extension": "wav",
        "content_type": "audio/wav",
        "description": "WAV PCM 24 kHz mono",
        "ffmpeg_args": ["-ac", "1", "-ar", "24000", "-c:a", "pcm_s16le"],
    },
    {
        "directory": "mp3",
        "extension": "mp3",
        "content_type": "audio/mpeg",
        "description": "MP3 mono",
        "ffmpeg_args": ["-codec:a", "libmp3lame", "-q:a", "3"],
    },
    {
        "directory": "flac",
        "extension": "flac",
        "content_type": "audio/flac",
        "description": "FLAC 22.05 kHz mono",
        "ffmpeg_args": ["-ac", "1", "-ar", "22050", "-c:a", "flac"],
    },
    {
        "directory": "ogg-opus",
        "extension": "ogg",
        "content_type": "audio/ogg",
        "description": "Ogg Opus mono",
        "ffmpeg_args": ["-c:a", "libopus", "-b:a", "32k"],
    },
]


def load_dotenv(path: Path) -> None:
    if not path.exists():
        return
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        os.environ.setdefault(key.strip(), value.strip())


def require_api_key() -> str:
    api_key = os.environ.get("TRANSCRIPTION_API_KEY") or os.environ.get("MIMO_API_KEY")
    if not api_key or api_key == "replace-me":
        raise SystemExit("missing TRANSCRIPTION_API_KEY or MIMO_API_KEY in environment/.env")
    return api_key


def call_mimo_tts(endpoint: str, api_key: str, model: str, voice: str, text: str) -> tuple[bytes, dict[str, Any]]:
    body = {
        "model": model,
        "messages": [{"role": "assistant", "content": text}],
        "audio": {"format": "wav", "voice": voice},
    }
    request = urllib.request.Request(
        endpoint,
        data=json.dumps(body, ensure_ascii=False).encode("utf-8"),
        headers={"Content-Type": "application/json", "api-key": api_key},
        method="POST",
    )
    try:
        raw = urllib.request.urlopen(request, timeout=120).read()
    except urllib.error.HTTPError as error:
        message = error.read().decode("utf-8", "replace")
        raise RuntimeError(f"MiMo TTS HTTP {error.code}: {message[:1000]}") from error
    response = json.loads(raw)
    audio = response["choices"][0]["message"]["audio"]
    data = audio["data"]
    if "," in data and data.split(",", 1)[0].startswith("data:"):
        data = data.split(",", 1)[1]
    return base64.b64decode(data), {
        "model": response.get("model"),
        "response_id": response.get("id"),
        "audio_id": audio.get("id"),
        "audio_transcript": audio.get("transcript"),
    }


def run_ffmpeg(input_path: Path, output_path: Path, ffmpeg_args: list[str]) -> None:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        [
            "ffmpeg",
            "-hide_banner",
            "-loglevel",
            "error",
            "-y",
            "-i",
            str(input_path),
            *ffmpeg_args,
            str(output_path),
        ],
        check=True,
    )


def fixture_sources(fixture_dir: Path) -> list[Path]:
    return sorted(path for path in fixture_dir.glob("*.txt") if path.name != "README.md")


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate MiMo TTS ASR audio fixtures")
    parser.add_argument("--fixture-dir", type=Path, default=DEFAULT_FIXTURE_DIR)
    parser.add_argument("--base-url", default=os.environ.get("TRANSCRIPTION_BASE_URL", DEFAULT_BASE_URL))
    parser.add_argument("--model", default=os.environ.get("MIMO_TTS_MODEL", DEFAULT_MODEL))
    parser.add_argument("--voice", default=os.environ.get("MIMO_TTS_VOICE", DEFAULT_VOICE))
    parser.add_argument("--sleep", type=float, default=0.2, help="Delay between MiMo API calls")
    args = parser.parse_args()

    load_dotenv(ROOT / ".env")
    api_key = require_api_key()
    endpoint = args.base_url.rstrip("/") + "/chat/completions"
    fixture_dir = args.fixture_dir
    tmp_dir = fixture_dir / ".generated"
    tmp_dir.mkdir(parents=True, exist_ok=True)

    sources = fixture_sources(fixture_dir)
    if not sources:
        raise SystemExit(f"no .txt fixture sources found in {fixture_dir}")

    manifest = []
    for source in sources:
        stem = source.stem
        text = source.read_text(encoding="utf-8")
        print(f"generating {stem} with {args.model}/{args.voice}")
        wav_bytes, metadata = call_mimo_tts(endpoint, api_key, args.model, args.voice, text)
        base_wav = tmp_dir / f"{stem}.mimo.wav"
        base_wav.write_bytes(wav_bytes)
        for fmt in FORMATS:
            output_path = fixture_dir / fmt["directory"] / f"{stem}.{fmt['extension']}"
            run_ffmpeg(base_wav, output_path, fmt["ffmpeg_args"])
            manifest.append(
                {
                    "id": stem,
                    "text_file": source.name,
                    "audio_file": f"{fmt['directory']}/{stem}.{fmt['extension']}",
                    "content_type": fmt["content_type"],
                    "description": fmt["description"],
                    "tts_model": metadata.get("model") or args.model,
                    "tts_voice": args.voice,
                    "response_id": metadata.get("response_id"),
                    "audio_id": metadata.get("audio_id"),
                    "audio_transcript": metadata.get("audio_transcript"),
                    "bytes": output_path.stat().st_size,
                }
            )
        time.sleep(args.sleep)

    (fixture_dir / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    shutil.rmtree(tmp_dir)
    print(f"wrote {fixture_dir / 'manifest.json'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
