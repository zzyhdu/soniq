import { useState } from 'react';

import { type UploadRecordingResponse } from '@soniq/api-client';

import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { RecordingUploadForm } from '@/components/RecordingUploadForm';
import { useUploadRecording } from '@/api/queries';

export function App() {
  const uploadRecordingMutation = useUploadRecording();
  const [latestUpload, setLatestUpload] = useState<UploadRecordingResponse | null>(null);

  return (
    <main className="min-h-screen bg-muted/30 px-6 py-10">
      <div className="mx-auto flex max-w-4xl flex-col gap-8">
        <section className="space-y-3">
          <Badge variant="secondary" className="w-fit">Local-first audio intelligence</Badge>
          <div className="space-y-2">
            <h1 className="text-4xl font-semibold tracking-tight text-balance">Soniq Web UI</h1>
            <p className="max-w-2xl text-muted-foreground">
              Upload audio, follow processing status, and review transcript and summary results from a browser.
            </p>
          </div>
        </section>

        <RecordingUploadForm
          onUpload={(input) => uploadRecordingMutation.mutateAsync(input)}
          onUploaded={setLatestUpload}
          isUploading={uploadRecordingMutation.isPending}
          error={uploadRecordingMutation.error instanceof Error ? uploadRecordingMutation.error.message : null}
        />

        {latestUpload !== null && (
          <Card>
            <CardHeader>
              <CardTitle>Upload created</CardTitle>
              <CardDescription>
                Status polling and results display will be added in the next tasks.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3 text-sm">
              <div>
                <span className="text-muted-foreground">Recording ID: </span>
                <span className="font-mono">{latestUpload.recording.id}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Processing enqueued: </span>
                <Badge variant={latestUpload.processing_enqueued ? 'default' : 'destructive'}>
                  {latestUpload.processing_enqueued ? 'yes' : 'no'}
                </Badge>
              </div>
            </CardContent>
          </Card>
        )}
      </div>
    </main>
  );
}
