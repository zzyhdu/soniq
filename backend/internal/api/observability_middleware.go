package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zzyhdu/soniq/backend/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type requestLogFieldsContextKey struct{}

type requestLogFields struct {
	userID      string
	workspaceID string
}

type apiErrorLog struct {
	code    apiErrorCode
	message string
}

// HTTPTracingConfig configures API request tracing.
type HTTPTracingConfig struct {
	Tracer     trace.Tracer
	Propagator propagation.TextMapPropagator
}

// RouterOptions configures optional API router observability behavior.
type RouterOptions struct {
	HTTPTracing HTTPTracingConfig
}

type responseLogRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	apiError    *apiErrorLog
}

func requestLoggingMiddleware(logger *slog.Logger, metrics *observability.Metrics) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			requestID, ok := observability.RequestIDFromHeader(r.Header.Get(observability.RequestIDHeader))
			if !ok {
				requestID = observability.NewRequestID()
			}
			w.Header().Set(observability.RequestIDHeader, requestID)

			logFields := &requestLogFields{}
			ctx := observability.ContextWithRequestID(r.Context(), requestID)
			ctx = context.WithValue(ctx, requestLogFieldsContextKey{}, logFields)
			request := r.WithContext(ctx)
			recorder := &responseLogRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(recorder, request)

			route := requestRoutePattern(request)
			duration := time.Since(startedAt)
			metrics.ObserveHTTPRequest(route, r.Method, recorder.status, duration)
			attrs := []any{
				slog.String("event", "http_request_completed"),
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("route", route),
				slog.Int("status", recorder.status),
				slog.Int64("duration_ms", duration.Milliseconds()),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			}
			if strings.TrimSpace(logFields.userID) != "" {
				attrs = append(attrs, slog.String("user_id", logFields.userID))
			}
			if strings.TrimSpace(logFields.workspaceID) != "" {
				attrs = append(attrs, slog.String("workspace_id", logFields.workspaceID))
			}
			logger.InfoContext(request.Context(), "http request completed", attrs...)

			if recorder.status >= http.StatusInternalServerError {
				errorCode := "unknown"
				errorMessage := "internal server error"
				if recorder.apiError != nil {
					errorCode = string(recorder.apiError.code)
					errorMessage = recorder.apiError.message
				}
				errorAttrs := []any{
					slog.String("event", "api_error"),
					slog.String("request_id", requestID),
					slog.String("route", route),
					slog.Int("status", recorder.status),
					slog.String("error_code", errorCode),
					slog.String("error", errorMessage),
				}
				if strings.TrimSpace(logFields.userID) != "" {
					errorAttrs = append(errorAttrs, slog.String("user_id", logFields.userID))
				}
				if strings.TrimSpace(logFields.workspaceID) != "" {
					errorAttrs = append(errorAttrs, slog.String("workspace_id", logFields.workspaceID))
				}
				logger.ErrorContext(request.Context(), "api error", errorAttrs...)
			}
		})
	}
}

func requestTracingMiddleware(config HTTPTracingConfig) func(http.Handler) http.Handler {
	if config.Tracer == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	propagator := config.Propagator
	if propagator == nil {
		propagator = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID, _ := observability.RequestIDFromContext(r.Context())
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := config.Tracer.Start(ctx, r.Method+" "+r.URL.Path,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.request.method", r.Method),
					attribute.String("url.path", r.URL.EscapedPath()),
					attribute.String("request_id", requestID),
				),
			)
			defer span.End()

			request := r.WithContext(ctx)
			recorder := &responseLogRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, request)

			route := requestRoutePattern(request)
			span.SetName(r.Method + " " + route)
			attrs := []attribute.KeyValue{
				attribute.String("http.route", route),
				attribute.Int("http.response.status_code", recorder.status),
			}
			if fields := requestLogFieldsFromContext(request.Context()); fields != nil {
				if strings.TrimSpace(fields.workspaceID) != "" {
					attrs = append(attrs, attribute.String("workspace_id", fields.workspaceID))
				}
			}
			if recordingID := strings.TrimSpace(chi.URLParam(request, "recording_id")); recordingID != "" {
				attrs = append(attrs, attribute.String("recording_id", recordingID))
			}
			span.SetAttributes(attrs...)
			if recorder.status >= http.StatusInternalServerError {
				message := http.StatusText(recorder.status)
				if recorder.apiError != nil && strings.TrimSpace(recorder.apiError.message) != "" {
					message = recorder.apiError.message
				}
				span.RecordError(errors.New(message))
				span.SetStatus(codes.Error, message)
			}
		})
	}
}

func (r *responseLogRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseLogRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(body)
}

func (r *responseLogRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		if !r.wroteHeader {
			r.WriteHeader(http.StatusOK)
		}
		flusher.Flush()
	}
}

func (r *responseLogRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *responseLogRecorder) RecordAPIError(status int, code apiErrorCode, message string) {
	r.apiError = &apiErrorLog{code: code, message: message}
	if observer, ok := r.ResponseWriter.(apiErrorObserver); ok {
		observer.RecordAPIError(status, code, message)
	}
}

func requestRoutePattern(r *http.Request) string {
	routeContext := chi.RouteContext(r.Context())
	if routeContext == nil {
		return "unmatched"
	}
	route := strings.TrimSpace(routeContext.RoutePattern())
	if route == "" {
		return "unmatched"
	}
	return route
}

func setRequestLogUserID(ctx context.Context, userID string) {
	if fields := requestLogFieldsFromContext(ctx); fields != nil {
		fields.userID = userID
	}
}

func setRequestLogWorkspaceID(ctx context.Context, workspaceID string) {
	if fields := requestLogFieldsFromContext(ctx); fields != nil {
		fields.workspaceID = workspaceID
	}
}

func requestLogFieldsFromContext(ctx context.Context) *requestLogFields {
	fields, _ := ctx.Value(requestLogFieldsContextKey{}).(*requestLogFields)
	return fields
}
