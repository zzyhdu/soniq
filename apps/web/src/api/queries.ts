import { type UploadRecordingInput, uploadRecording } from '@soniq/api-client';
import { useMutation } from '@tanstack/react-query';

export function useUploadRecording() {
  return useMutation({
    mutationFn: (input: UploadRecordingInput) => uploadRecording(input),
  });
}
