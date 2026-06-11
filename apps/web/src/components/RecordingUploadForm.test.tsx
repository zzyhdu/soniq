import '@testing-library/jest-dom/vitest';

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { RecordingUploadForm } from './RecordingUploadForm';

afterEach(() => {
  cleanup();
});

function renderForm(overrides: Partial<React.ComponentProps<typeof RecordingUploadForm>> = {}) {
  const onUpload = vi.fn<React.ComponentProps<typeof RecordingUploadForm>['onUpload']>();
  const onUploaded = vi.fn<React.ComponentProps<typeof RecordingUploadForm>['onUploaded']>();

  render(
    <RecordingUploadForm
      onUpload={onUpload}
      onUploaded={onUploaded}
      isUploading={false}
      error={null}
      {...overrides}
    />,
  );

  return { onUpload, onUploaded };
}

describe('RecordingUploadForm', () => {
  it('disables submit until an audio file is selected', async () => {
    const user = userEvent.setup();
    renderForm();

    const submit = screen.getByRole('button', { name: /upload recording/i });
    expect(submit).toBeDisabled();

    await user.upload(screen.getByLabelText(/audio file/i), audioFile());

    expect(submit).toBeEnabled();
  });

  it('passes workflow type, title, language, and audio to upload', async () => {
    const user = userEvent.setup();
    const { onUpload } = renderForm();
    const audio = audioFile();

    await user.clear(screen.getByLabelText(/title/i));
    await user.type(screen.getByLabelText(/title/i), 'Weekly sync');
    await user.selectOptions(screen.getByLabelText(/workflow type/i), 'interview');
    await user.clear(screen.getByLabelText(/language/i));
    await user.type(screen.getByLabelText(/language/i), 'en');
    await user.upload(screen.getByLabelText(/audio file/i), audio);
    await user.click(screen.getByRole('button', { name: /upload recording/i }));

    expect(onUpload).toHaveBeenCalledWith({
      title: 'Weekly sync',
      workflow_type: 'interview',
      language: 'en',
      audio,
    });
  });

  it('calls onUploaded after a successful upload', async () => {
    const user = userEvent.setup();
    const response = {
      recording: {
        id: 'rec-1',
        workspace_id: 'wsp_default',
        title: 'Weekly sync',
        status: 'uploaded' as const,
        workflow_type: 'meeting' as const,
        language: 'zh',
        created_at: '2026-06-07T00:00:00Z',
        updated_at: '2026-06-07T00:00:00Z',
      },
      processing_enqueued: true,
    };
    const { onUpload, onUploaded } = renderForm();
    onUpload.mockResolvedValue(response);

    await user.upload(screen.getByLabelText(/audio file/i), audioFile());
    await user.click(screen.getByRole('button', { name: /upload recording/i }));

    expect(onUploaded).toHaveBeenCalledWith(response);
  });

  it('shows upload errors', () => {
    renderForm({ error: 'Upload failed' });

    expect(screen.getByText('Upload failed')).toBeInTheDocument();
  });
});

function audioFile() {
  return new File(['audio-bytes'], 'meeting.mp3', { type: 'audio/mpeg' });
}
