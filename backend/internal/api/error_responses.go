package api

import (
	"encoding/json"
	"net/http"
)

type apiErrorCode string

const (
	errorCodeUnauthenticated      apiErrorCode = "unauthenticated"
	errorCodeInvalidCredentials   apiErrorCode = "invalid_credentials"
	errorCodeInvalidCSRFToken     apiErrorCode = "invalid_csrf_token"
	errorCodeUserAlreadyExists    apiErrorCode = "user_already_exists"
	errorCodeValidationFailed     apiErrorCode = "validation_failed"
	errorCodeForbidden            apiErrorCode = "forbidden"
	errorCodeNotFound             apiErrorCode = "not_found"
	errorCodeMethodNotAllowed     apiErrorCode = "method_not_allowed"
	errorCodeRequestTooLarge      apiErrorCode = "request_too_large"
	errorCodeUnsupportedMediaType apiErrorCode = "unsupported_media_type"
	errorCodeConflict             apiErrorCode = "conflict"
	errorCodeRateLimited          apiErrorCode = "rate_limited"
	errorCodeInternalError        apiErrorCode = "internal_error"
	errorCodeServiceUnavailable   apiErrorCode = "service_unavailable"
)

type apiErrorResponse struct {
	Code    apiErrorCode `json:"code"`
	Message string       `json:"message"`
	Status  int          `json:"status"`
}

func writeAPIError(w http.ResponseWriter, status int, code apiErrorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiErrorResponse{
		Code:    code,
		Message: message,
		Status:  status,
	})
}

func writeMethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeAPIError(w, http.StatusMethodNotAllowed, errorCodeMethodNotAllowed, "method not allowed")
}
