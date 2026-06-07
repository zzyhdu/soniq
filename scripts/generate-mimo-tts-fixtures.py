#!/usr/bin/env python3
"""Generate synthetic ASR audio fixtures with Xiaomi MiMo TTS.

The script reads source `.txt` files from `testdata/asr/mimo-tts`, parses
speaker-prefixed dialogue lines, calls MiMo TTS (`mimo-v2.5-tts`) per utterance,
and concatenates the utterances into multi-speaker dialogue-style audio before
writing multiple input formats for upload/normalization smoke tests.

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
DEFAULT_MODEL = "mimo-v2.5-tts"
DEFAULT_VOICE = "茉莉"
DEFAULT_BASE_URL = "https://api.xiaomimimo.com/v1"
DEFAULT_UTTERANCE_GAP_SECONDS = 0.45

SPEAKER_PROFILES = [
    {
        "profile": "speaker_a",
        "voice": "茉莉",
        "style": "年轻女性产品经理，会议语气自然，声音明亮清晰，标准自然语速",
    },
    {
        "profile": "speaker_b",
        "voice": "苏打",
        "style": "年轻男性工程师，技术讨论语气，声音沉稳，标准自然语速，咬字清楚",
    },
    {
        "profile": "speaker_c",
        "voice": "白桦",
        "style": "成熟男性主持人，会议主持语气，声音厚实稳重，标准自然语速",
    },
    {
        "profile": "speaker_d",
        "voice": "冰糖",
        "style": "温和女性同事，口语自然，声音柔和，标准自然语速",
    },
]


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


def parse_dialogue(text: str) -> list[tuple[str, str]]:
    utterances = []
    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line:
            continue
        if "：" in line:
            speaker, utterance = line.split("：", 1)
        elif ":" in line:
            speaker, utterance = line.split(":", 1)
        else:
            speaker, utterance = "旁白", line
        utterance = utterance.strip()
        if utterance:
            utterances.append((speaker.strip(), utterance))
    return utterances


def assign_speaker_profiles(utterances: list[tuple[str, str]], fallback_voice: str) -> dict[str, dict[str, str]]:
    profiles = {}
    for speaker, _ in utterances:
        if speaker in profiles:
            continue
        profile = SPEAKER_PROFILES[len(profiles) % len(SPEAKER_PROFILES)].copy()
        if not profile.get("voice"):
            profile["voice"] = fallback_voice
        profiles[speaker] = profile
    return profiles


def styled_utterance(speaker: str, text: str, profiles: dict[str, dict[str, str]]) -> tuple[str, str, str, dict[str, str]]:
    profile = profiles[speaker]
    voice = profile["voice"]
    instruction = (
        f"请用以下人物音色和表演方式合成语音。角色：{speaker}。"
        f"声音设定：{profile['style']}。"
        "这是多人会议对话中的一句话，只合成 assistant 消息中的内容，不要朗读角色名。"
        "请使用标准自然会议语速，逐字完整读完，不要省略、不要吞掉句尾、不要压缩成短音节。"
    )
    return voice, text, instruction, profile


def call_mimo_tts(
    endpoint: str,
    api_key: str,
    model: str,
    voice: str,
    text: str,
    instruction: str | None = None,
) -> tuple[bytes, dict[str, Any]]:
    messages = []
    if instruction:
        messages.append({"role": "user", "content": instruction})
    messages.append({"role": "assistant", "content": text})
    body = {
        "model": model,
        "messages": messages,
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



def make_silence(path: Path, seconds: float, sample_rate: int = 24000) -> None:
    subprocess.run(
        [
            "ffmpeg",
            "-hide_banner",
            "-loglevel",
            "error",
            "-y",
            "-f",
            "lavfi",
            "-i",
            f"anullsrc=r={sample_rate}:cl=mono",
            "-t",
            f"{seconds:.3f}",
            "-c:a",
            "pcm_s16le",
            str(path),
        ],
        check=True,
    )


def concat_wavs(parts: list[Path], output_path: Path) -> None:
    concat_file = output_path.with_suffix(".concat.txt")
    concat_file.write_text("".join(f"file '{part.as_posix()}'\n" for part in parts), encoding="utf-8")
    try:
        subprocess.run(
            [
                "ffmpeg",
                "-hide_banner",
                "-loglevel",
                "error",
                "-y",
                "-f",
                "concat",
                "-safe",
                "0",
                "-i",
                str(concat_file),
                "-ac",
                "1",
                "-ar",
                "24000",
                "-c:a",
                "pcm_s16le",
                str(output_path),
            ],
            check=True,
        )
    finally:
        concat_file.unlink(missing_ok=True)


def synthesize_dialogue(
    endpoint: str,
    api_key: str,
    model: str,
    fallback_voice: str,
    source: Path,
    tmp_dir: Path,
    gap_seconds: float,
) -> tuple[Path, dict[str, Any]]:
    text = source.read_text(encoding="utf-8")
    utterances = parse_dialogue(text)
    if not utterances:
        raise RuntimeError(f"no utterances parsed from {source}")

    profiles = assign_speaker_profiles(utterances, fallback_voice)
    parts = []
    metadata = {
        "speaker_profiles": profiles,
        "utterances": [],
    }
    for index, (speaker, utterance) in enumerate(utterances):
        voice, styled_text, instruction, profile = styled_utterance(speaker, utterance, profiles)
        wav_bytes, utterance_metadata = call_mimo_tts(endpoint, api_key, model, voice, styled_text, instruction)
        utterance_path = tmp_dir / f"{source.stem}.{index:03d}.{speaker}.wav"
        utterance_path.write_bytes(wav_bytes)
        parts.append(utterance_path)
        metadata["utterances"].append(
            {
                "speaker": speaker,
                "profile": profile["profile"],
                "voice": voice,
                "style": profile["style"],
                "instruction": instruction,
                "text": utterance,
                "response_id": utterance_metadata.get("response_id"),
                "audio_id": utterance_metadata.get("audio_id"),
            }
        )
        if index != len(utterances) - 1:
            silence_path = tmp_dir / f"{source.stem}.{index:03d}.silence.wav"
            make_silence(silence_path, gap_seconds)
            parts.append(silence_path)

    base_wav = tmp_dir / f"{source.stem}.dialogue.wav"
    concat_wavs(parts, base_wav)
    metadata["model"] = model
    metadata["utterance_count"] = len(utterances)
    return base_wav, metadata


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
    parser.add_argument("--utterance-gap", type=float, default=DEFAULT_UTTERANCE_GAP_SECONDS)
    parser.add_argument("--only", action="append", default=[], help="Generate only the named fixture stem; repeatable")
    parser.add_argument("--sleep", type=float, default=0.2, help="Delay between MiMo API calls")
    args = parser.parse_args()

    load_dotenv(ROOT / ".env")
    api_key = require_api_key()
    endpoint = args.base_url.rstrip("/") + "/chat/completions"
    fixture_dir = args.fixture_dir
    tmp_dir = fixture_dir / ".generated"
    tmp_dir.mkdir(parents=True, exist_ok=True)

    sources = fixture_sources(fixture_dir)
    if args.only:
        allowed = set(args.only)
        sources = [source for source in sources if source.stem in allowed]
    if not sources:
        raise SystemExit(f"no .txt fixture sources found in {fixture_dir}")

    existing_manifest = []
    manifest_path = fixture_dir / "manifest.json"
    if args.only and manifest_path.exists():
        existing_manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    regenerated_ids = {source.stem for source in sources}
    manifest = [item for item in existing_manifest if item.get("id") not in regenerated_ids]
    for source in sources:
        stem = source.stem
        print(f"generating dialogue {stem} with {args.model}")
        base_wav, metadata = synthesize_dialogue(endpoint, api_key, args.model, args.voice, source, tmp_dir, args.utterance_gap)
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
                    "utterance_count": metadata.get("utterance_count"),
                    "utterance_gap_seconds": args.utterance_gap,
                    "speaker_profiles": metadata.get("speaker_profiles", {}),
                    "utterances": metadata.get("utterances", []),
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
