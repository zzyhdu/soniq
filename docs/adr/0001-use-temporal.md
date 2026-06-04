# ADR-0001: Use Temporal for durable workflow orchestration

## Status

Accepted.

## Context

The product processes audio through long-running, failure-prone steps:

- audio probing and normalization
- long transcription jobs
- third-party provider polling or webhooks
- LLM summarization
- persistence and notification

A simple queue would require us to build our own retry, state machine, timeout, resume, cancellation, workflow history, and signal handling.

## Decision

Use Temporal as the workflow orchestration layer. Default to self-hosted Temporal for open-source/local deployments, with Temporal Cloud as an optional production choice where available.

## Consequences

Positive:

- Durable execution.
- Built-in retry and timeout policies.
- Long-running workflow support.
- Signals and queries.
- Strong learning value.
- Good fit for enterprise-grade processing.

Negative:

- Additional infrastructure component.
- Workflow determinism rules must be learned.
- More complex than a simple job queue for small deployments.

## Notes

All external I/O must be placed in activities, not workflow code.
