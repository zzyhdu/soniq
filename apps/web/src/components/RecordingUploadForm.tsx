import { type FormEvent, useState } from 'react';

import { type UploadRecordingInput, type UploadRecordingResponse, type WorkflowType } from '@soniq/api-client';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select } from '@/components/ui/select';

export type RecordingUploadFormProps = {
  onUpload: (input: UploadRecordingInput) => Promise<UploadRecordingResponse> | UploadRecordingResponse;
  onUploaded: (response: UploadRecordingResponse) => void;
  isUploading: boolean;
  error: string | null;
};

export function RecordingUploadForm({ onUpload, onUploaded, isUploading, error }: RecordingUploadFormProps) {
  const [title, setTitle] = useState('');
  const [workflowType, setWorkflowType] = useState<WorkflowType>('meeting');
  const [language, setLanguage] = useState('zh');
  const [audio, setAudio] = useState<File | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (audio === null) {
      return;
    }

    const response = await onUpload({
      audio,
      workflow_type: workflowType,
      title: title.trim() || undefined,
      language: language.trim() || undefined,
    });
    onUploaded(response);
  }

  return (
    <Card>
      <CardHeader>
        <div className="space-y-1.5">
          <CardTitle>Upload recording</CardTitle>
          <CardDescription>Select audio and start Soniq processing.</CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        <form className="grid gap-4 md:grid-cols-2" onSubmit={handleSubmit}>
          <div className="space-y-2">
            <Label htmlFor="recording-title">Title</Label>
            <Input
              id="recording-title"
              placeholder="Weekly standup"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="workflow-type">Workflow type</Label>
            <Select
              id="workflow-type"
              value={workflowType}
              onChange={(event) => setWorkflowType(event.target.value as WorkflowType)}
            >
              <option value="meeting">Meeting</option>
              <option value="lecture">Lecture</option>
              <option value="interview">Interview</option>
              <option value="memo">Memo</option>
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="recording-language">Language</Label>
            <Input
              id="recording-language"
              value={language}
              onChange={(event) => setLanguage(event.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="recording-audio">Audio file</Label>
            <Input
              id="recording-audio"
              type="file"
              accept="audio/*"
              onChange={(event) => setAudio(event.target.files?.[0] ?? null)}
            />
          </div>

          {error !== null && (
            <p className="text-destructive text-sm md:col-span-2" role="alert">
              {error}
            </p>
          )}

          <div className="flex items-center gap-3 md:col-span-2">
            <Button type="submit" disabled={audio === null || isUploading}>
              {isUploading ? 'Uploading…' : 'Upload recording'}
            </Button>
            <span className="text-muted-foreground text-sm">
              Processing starts after the audio upload succeeds.
            </span>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
