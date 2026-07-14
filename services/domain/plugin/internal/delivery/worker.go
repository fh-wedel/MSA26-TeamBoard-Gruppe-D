package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sony/gobreaker"
	"github.com/teamboard/services/domain/plugin/internal/domain"
)

const maxResponseBodyBytes = 1 << 20 // 1 MB

type DeliveryResult struct {
	Success    bool
	StatusCode *int
	Body       *string
	Err        *string
	DurationMs int
}

type Worker struct {
	repo      domain.Repository
	client    *http.Client
	breakers  *BreakerRegistry
	userAgent string
	logger    *slog.Logger
}

func NewWorker(repo domain.Repository, client *http.Client, breakers *BreakerRegistry, userAgent string, logger *slog.Logger) *Worker {
	return &Worker{
		repo:      repo,
		client:    client,
		breakers:  breakers,
		userAgent: userAgent,
		logger:    logger,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for {
				delivered, err := w.DeliverNext(ctx)
				if err != nil {
					w.logger.Warn("delivery iteration failed", "err", err)
					break
				}
				if !delivered {
					break
				}
			}
		}
	}
}

func (w *Worker) DeliverNext(ctx context.Context) (bool, error) {
	d, err := w.repo.PickNextPendingDelivery(ctx)
	if errors.Is(err, domain.ErrNoPendingDelivery) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	wh, err := w.repo.GetWebhook(ctx, d.WebhookID)
	if err != nil || !wh.Active {
		reason := "webhook inactive or deleted"
		if err != nil {
			reason = err.Error()
		}
		return true, w.repo.MarkDeliveryDead(ctx, d.ID, reason)
	}

	breaker := w.breakers.Get(wh.TargetURL)
	result, breakerErr := breaker.Execute(func() (interface{}, error) {
		r := w.httpDeliver(ctx, wh, d)
		if !r.Success {
			return r, fmt.Errorf("delivery failed")
		}
		return r, nil
	})

	var deliveryResult *DeliveryResult
	if breakerErr != nil {
		if errors.Is(breakerErr, gobreaker.ErrOpenState) || errors.Is(breakerErr, gobreaker.ErrTooManyRequests) {
			// Circuit is open — reschedule without incrementing attempt
			next := time.Now().Add(30 * time.Second)
			return true, w.repo.RescheduleDelivery(ctx, d.ID, next, nil, nil, nil, 0)
		}
		// The inner function returned an error — result holds the DeliveryResult
		if r, ok := result.(*DeliveryResult); ok {
			deliveryResult = r
		} else {
			errStr := breakerErr.Error()
			deliveryResult = &DeliveryResult{Success: false, Err: &errStr}
		}
	} else {
		deliveryResult = result.(*DeliveryResult)
	}

	return true, w.applyResult(ctx, d, deliveryResult)
}

func (w *Worker) httpDeliver(ctx context.Context, wh *domain.Webhook, d *domain.Delivery) *DeliveryResult {
	body, err := json.Marshal(d.Payload)
	if err != nil {
		errStr := "payload marshal failed: " + err.Error()
		return &DeliveryResult{Success: false, Err: &errStr}
	}

	ts := time.Now().Unix()
	sig := SignPayload(wh.Secret, body, ts)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.TargetURL, bytes.NewReader(body))
	if err != nil {
		errStr := err.Error()
		return &DeliveryResult{Success: false, Err: &errStr}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", w.userAgent)
	req.Header.Set("X-TeamBoard-Event", d.EventType)
	req.Header.Set("X-TeamBoard-Event-Id", d.EventID)
	req.Header.Set("X-TeamBoard-Delivery-Id", d.ID.String())
	req.Header.Set("X-TeamBoard-Signature", sig)
	req.Header.Set("X-TeamBoard-Timestamp", fmt.Sprintf("%d", ts))

	start := time.Now()
	resp, err := w.client.Do(req)
	durationMs := int(time.Since(start).Milliseconds())

	if err != nil {
		errStr := err.Error()
		return &DeliveryResult{Success: false, Err: &errStr, DurationMs: durationMs}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	respBodyStr := string(respBody)
	statusCode := resp.StatusCode

	success := statusCode >= 200 && statusCode < 300
	result := &DeliveryResult{
		Success:    success,
		StatusCode: &statusCode,
		Body:       &respBodyStr,
		DurationMs: durationMs,
	}
	if !success {
		errStr := fmt.Sprintf("HTTP %d", statusCode)
		result.Err = &errStr
	}
	return result
}

func (w *Worker) applyResult(ctx context.Context, d *domain.Delivery, result *DeliveryResult) error {
	if result.Success {
		body := ""
		if result.Body != nil {
			body = *result.Body
		}
		statusCode := 200
		if result.StatusCode != nil {
			statusCode = *result.StatusCode
		}
		return w.repo.MarkDeliveryDelivered(ctx, d.ID, statusCode, body, result.DurationMs)
	}

	// Check permanent failure conditions
	if result.StatusCode != nil && IsPermanentFailure(*result.StatusCode) {
		reason := fmt.Sprintf("HTTP %d", *result.StatusCode)
		if err := w.repo.MarkDeliveryDead(ctx, d.ID, reason); err != nil {
			return err
		}
		if *result.StatusCode == http.StatusGone {
			_ = w.repo.SetWebhookActive(ctx, d.WebhookID, false)
		}
		return nil
	}

	nextAt, ok := ComputeNextAttempt(d.AttemptCount + 1)
	if !ok {
		reason := "max attempts reached"
		if result.Err != nil {
			reason = *result.Err
		}
		return w.repo.MarkDeliveryDead(ctx, d.ID, reason)
	}

	return w.repo.RescheduleDelivery(ctx, d.ID, nextAt, result.StatusCode, result.Body, result.Err, result.DurationMs)
}

// Ensure Repository has SetWebhookActive — add a local adapter call.
// This is called on 410 Gone to auto-disable the webhook.
func init() {
	_ = uuid.Nil // import used
}
