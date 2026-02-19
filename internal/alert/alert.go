package alert

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/hazz-dev/servprobe/internal/checker"
)

// Alerter sends webhook notifications on service state changes.
type Alerter struct {
	webhookURL string
	cooldown   time.Duration
	client     *http.Client
	lastAlert  map[string]time.Time
	mu         sync.Mutex
	logger     *slog.Logger
}

// New creates a new Alerter. Pass nil logger to use the default logger.
func New(webhookURL string, cooldown time.Duration, logger *slog.Logger) *Alerter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Alerter{
		webhookURL: webhookURL,
		cooldown:   cooldown,
		client:     &http.Client{Timeout: 10 * time.Second},
		lastAlert:  make(map[string]time.Time),
		logger:     logger,
	}
}

type webhookPayload struct {
	Service        string `json:"service"`
	Status         string `json:"status"`
	PreviousStatus string `json:"previous_status"`
	Error          string `json:"error"`
	ResponseTimeMs int64  `json:"response_time_ms"`
	CheckedAt      string `json:"checked_at"`
	Source         string `json:"source"`
}

// Notify sends a webhook if the service state has changed and the cooldown has elapsed.
func (a *Alerter) Notify(result checker.CheckResult, previousStatus *checker.Status) {
	// No previous status means first check — skip.
	if previousStatus == nil {
		return
	}
	// No state change — skip.
	if result.Status == *previousStatus {
		return
	}

	// Check cooldown.
	a.mu.Lock()
	last, exists := a.lastAlert[result.ServiceName]
	if exists && time.Since(last) < a.cooldown {
		a.mu.Unlock()
		a.logger.Info("alert suppressed by cooldown", "service", result.ServiceName)
		return
	}
	a.lastAlert[result.ServiceName] = time.Now()
	a.mu.Unlock()

	// Send asynchronously so Notify doesn't block the scheduler.
	go a.send(result, string(*previousStatus))
}

func (a *Alerter) send(result checker.CheckResult, prevStatus string) {
	payload := webhookPayload{
		Service:        result.ServiceName,
		Status:         string(result.Status),
		PreviousStatus: prevStatus,
		Error:          result.Error,
		ResponseTimeMs: result.ResponseTime.Milliseconds(),
		CheckedAt:      result.CheckedAt.UTC().Format(time.RFC3339),
		Source:         "servprobe",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		a.logger.Error("marshaling webhook payload", "service", result.ServiceName, "error", err)
		return
	}

	resp, err := a.client.Post(a.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		a.logger.Error("sending webhook", "service", result.ServiceName, "url", a.webhookURL, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		a.logger.Warn("webhook returned non-2xx status",
			"service", result.ServiceName,
			"status", resp.StatusCode,
		)
	}
}
