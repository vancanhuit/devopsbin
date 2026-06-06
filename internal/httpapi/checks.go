package httpapi

import (
	"context"
	"log/slog"
	"time"
)

// Pinger is implemented by dependencies that can verify their connectivity,
// such as the Postgres store and Redis cache.
type Pinger interface {
	Ping(ctx context.Context) error
}

// pingFailedMessage is the generic message reported by PingCheck on failure.
// The underlying error (which may embed host, port, and user) is logged rather
// than returned so probe responses do not disclose connection details.
const pingFailedMessage = "dependency unavailable"

// PingCheck adapts a Pinger into a Check. It reports ok when Ping succeeds and
// error otherwise. The detailed failure is logged via slog and a generic
// message is returned so probe responses do not leak connection details. Each
// ping is bounded by timeout so a probe against an unreachable dependency fails
// fast instead of blocking on a connection attempt.
func PingCheck(p Pinger, timeout time.Duration) Check {
	return func(ctx context.Context) DependencyCheck {
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		if err := p.Ping(ctx); err != nil {
			slog.ErrorContext(ctx, "dependency ping failed", "error", err)
			msg := pingFailedMessage
			return DependencyCheck{Status: DependencyCheckStatusError, Message: &msg}
		}
		return DependencyCheck{Status: DependencyCheckStatusOk}
	}
}
