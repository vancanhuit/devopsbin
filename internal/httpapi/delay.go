package httpapi

import (
	"context"
	"time"
)

// maxDelay bounds the artificial latency the /delay endpoint will introduce.
// Requests for longer delays are clamped to this cap.
const maxDelay = 10 * time.Second

// clampDelay converts a non-negative seconds value into a duration bounded by
// maxDelay. Callers must reject negative input before calling.
func clampDelay(seconds float64) time.Duration {
	delay := time.Duration(seconds * float64(time.Second))
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

// GetDelay implements the /delay/{seconds} endpoint, waiting for the requested
// number of seconds (clamped to maxDelay) before responding. Negative values
// are rejected with a 400. The wait respects request cancellation and the
// server's request timeout.
func (s *Server) GetDelay(ctx context.Context, request GetDelayRequestObject) (GetDelayResponseObject, error) {
	if request.Seconds < 0 {
		return GetDelay400JSONResponse{Error: "delay seconds must not be negative"}, nil
	}

	delay := clampDelay(request.Seconds)

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}

	return GetDelay200JSONResponse{Delay: delay.Seconds()}, nil
}
