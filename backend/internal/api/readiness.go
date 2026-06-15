package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	readinessStatusReady    = "ready"
	readinessStatusNotReady = "not_ready"

	readinessCheckStatusOK    = "ok"
	readinessCheckStatusError = "error"
)

var defaultReadinessCheckNames = []string{"postgres", "migrations", "temporal", "object_storage"}

// ReadinessChecker reports whether the API process can serve real traffic.
type ReadinessChecker interface {
	CheckReadiness(ctx context.Context) ReadinessReport
}

// ReadinessReport contains dependency check results for /readyz.
type ReadinessReport struct {
	Checks map[string]ReadinessCheck
}

// ReadinessCheck contains one dependency check result.
type ReadinessCheck struct {
	OK    bool
	Error string
}

// ReadinessCheckOK builds a successful dependency check result.
func ReadinessCheckOK() ReadinessCheck {
	return ReadinessCheck{OK: true}
}

// ReadinessCheckFailed builds a failed dependency check result.
func ReadinessCheckFailed(message string) ReadinessCheck {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "check failed"
	}
	return ReadinessCheck{Error: message}
}

type staticReadinessChecker struct {
	report ReadinessReport
}

type readyzResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
	Errors map[string]string `json:"errors,omitempty"`
}

func readyzHandler(checker ReadinessChecker) http.HandlerFunc {
	if checker == nil {
		checker = staticReadinessChecker{report: ReadinessReport{
			Checks: map[string]ReadinessCheck{
				"postgres":       ReadinessCheckOK(),
				"migrations":     ReadinessCheckOK(),
				"temporal":       ReadinessCheckOK(),
				"object_storage": ReadinessCheckOK(),
			},
		}}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, http.MethodGet)
			return
		}

		report := checker.CheckReadiness(r.Context())
		response := readinessResponseFromReport(report)
		status := http.StatusOK
		if response.Status != readinessStatusReady {
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	}
}

func (c staticReadinessChecker) CheckReadiness(context.Context) ReadinessReport {
	return c.report
}

func readinessResponseFromReport(report ReadinessReport) readyzResponse {
	checks := make(map[string]string, len(report.Checks))
	errorsByCheck := map[string]string{}
	ready := true

	checkNames := defaultReadinessCheckNames
	if len(report.Checks) == 0 {
		checkNames = []string{"readiness"}
	}
	for _, name := range checkNames {
		check, ok := report.Checks[name]
		if !ok {
			checks[name] = readinessCheckStatusError
			errorsByCheck[name] = "check did not run"
			ready = false
			continue
		}
		if check.OK {
			checks[name] = readinessCheckStatusOK
			continue
		}
		checks[name] = readinessCheckStatusError
		errorsByCheck[name] = readinessErrorMessage(check.Error)
		ready = false
	}

	status := readinessStatusReady
	if !ready {
		status = readinessStatusNotReady
	}
	response := readyzResponse{
		Status: status,
		Checks: checks,
	}
	if len(errorsByCheck) > 0 {
		response.Errors = errorsByCheck
	}
	return response
}

func readinessErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "check failed"
	}
	return message
}
