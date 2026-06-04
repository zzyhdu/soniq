# Security and Privacy

Audio recordings and transcripts are sensitive data. Privacy and compliance must shape the architecture from the beginning.

## Privacy principles

- Make external provider usage explicit.
- Support private/offline deployment.
- Support deleting original audio after transcription.
- Keep audit logs for sensitive operations.
- Avoid storing secrets in plaintext.
- Treat generated transcripts and summaries as sensitive, not just the audio file.

## Retention controls

Soniq should support workspace-level retention policies:

```yaml
privacy:
  delete_original_audio_after_transcription: false
  retain_original_audio_days: 30
  retain_normalized_audio_days: 7
  retain_transcript_days: 365
  retain_summary_days: 365
  allow_external_model_providers: true
```

For offline/private deployments:

```yaml
privacy:
  allow_external_model_providers: false
```

When external providers are disabled, provider resolution must reject OpenAI/Anthropic/Gemini/etc. and only allow local or approved domestic/private providers.

## Secret management

Initial supported mode:

- Environment variables.

Future modes:

- Database encrypted workspace secrets.
- Kubernetes Secrets.
- HashiCorp Vault.
- AWS Secrets Manager.
- Aliyun KMS / Secrets Manager.

## Audit events

Minimum audit events:

- recording uploaded
- workflow started/completed/failed/cancelled
- transcript generated
- summary generated
- artifact deleted
- retention policy changed
- provider configuration changed

## Data deletion

Deletion should cover:

- database rows
- object storage artifacts
- workflow metadata where possible
- derived transcript/summary artifacts if requested

Temporal history may retain workflow metadata for its retention window, so user-facing deletion docs must explain this clearly.

## Network boundaries

Domestic/private deployments may need to prevent data egress. Soniq should support an allowlist of provider endpoints and a mode that denies all external AI calls.
