package storage

import "path/filepath"

// NormalizedAudioObjectKey returns the deterministic derived object key for a recording's normalized audio artifact.
func NormalizedAudioObjectKey(originalKey string) (string, error) {
	cleaned, err := cleanObjectKey(originalKey)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(filepath.Dir(cleaned), "normalized.wav")), nil
}
