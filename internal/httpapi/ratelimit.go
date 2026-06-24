package httpapi

import (
	"context"

	"github.com/vancanhuit/devopsbin/internal/ratelimit"
)

// GetRatelimit counts the request against a per-client-IP fixed-window limiter
// and reports the remaining allowance. While within the limit it returns 200;
// once the limit is exceeded it returns 429 with a Retry-After header. Every
// response carries the standard RateLimit-Limit, RateLimit-Remaining, and
// RateLimit-Reset headers. When no limiter is configured the request is always
// allowed.
func (s *Server) GetRatelimit(ctx context.Context, _ GetRatelimitRequestObject) (GetRatelimitResponseObject, error) {
	if s.ratelimiter == nil {
		body := RateLimitResponse{Limit: 0, Remaining: 0, Reset: 0}
		return GetRatelimit200JSONResponse{Body: body}, nil
	}

	ip := originIP(requestFrom(ctx))
	result := s.ratelimiter.Allow(ctx, ip)

	limit := int32(result.Limit)
	remaining := int32(result.Remaining)
	reset := int32(ratelimit.Seconds(result.Reset))

	if !result.Allowed {
		retryAfter := int32(ratelimit.Seconds(result.RetryAfter))
		return GetRatelimit429JSONResponse{
			Body: ErrorResponse{Error: "rate limit exceeded"},
			Headers: GetRatelimit429ResponseHeaders{
				RateLimitLimit:     &limit,
				RateLimitRemaining: &remaining,
				RateLimitReset:     &reset,
				RetryAfter:         &retryAfter,
			},
		}, nil
	}

	return GetRatelimit200JSONResponse{
		Body: RateLimitResponse{
			Limit:     limit,
			Remaining: remaining,
			Reset:     reset,
		},
		Headers: GetRatelimit200ResponseHeaders{
			RateLimitLimit:     &limit,
			RateLimitRemaining: &remaining,
			RateLimitReset:     &reset,
		},
	}, nil
}
