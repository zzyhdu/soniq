# ADR-0002: Use Go for API server and Temporal worker

## Status

Accepted.

## Context

The project aims to be open-source, enterprise-ready, high-concurrency, and deployment-friendly. The author also wants to learn Go as part of the project.

## Decision

Use Go for:

- API server
- Temporal workflow definitions
- Temporal activities where practical
- provider abstractions

Use TypeScript for frontend. Use Python later for optional heavy ML/audio workers when faster-whisper, pyannote, or torch-based workflows are needed.

## Consequences

Positive:

- Single-binary backend deployment.
- Low runtime overhead.
- Mature Temporal Go SDK.
- Good concurrency model.
- Good fit for infrastructure-style open-source projects.

Negative:

- Slower UI/API iteration than full TypeScript stack.
- Less convenient type sharing with frontend.
- ML ecosystem weaker than Python.

## Notes

Do not force ML-heavy code into Go. Add Python workers when local ASR/diarization requires it.
