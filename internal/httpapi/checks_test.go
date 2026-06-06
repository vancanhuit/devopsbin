package httpapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/httpapi"
)

// stubPinger is a Pinger whose Ping returns err and records the context it was
// called with, so tests can assert on timeout propagation.
type stubPinger struct {
	err    error
	gotCtx context.Context
}

func (s *stubPinger) Ping(ctx context.Context) error {
	s.gotCtx = ctx
	return s.err
}

func TestPingCheck_OK(t *testing.T) {
	check := httpapi.PingCheck(&stubPinger{}, time.Second)

	got := check(context.Background())

	if got.Status != httpapi.DependencyCheckStatusOk {
		t.Fatalf("status = %q, want %q", got.Status, httpapi.DependencyCheckStatusOk)
	}
	if got.Message != nil {
		t.Fatalf("message = %q, want nil", *got.Message)
	}
}

func TestPingCheck_Error(t *testing.T) {
	check := httpapi.PingCheck(&stubPinger{err: errors.New("connection refused")}, time.Second)

	got := check(context.Background())

	if got.Status != httpapi.DependencyCheckStatusError {
		t.Fatalf("status = %q, want %q", got.Status, httpapi.DependencyCheckStatusError)
	}
	// The generic message must be returned so the underlying error (which can
	// embed host/port/user) is not disclosed in probe responses.
	if got.Message == nil || *got.Message != "dependency unavailable" {
		t.Fatalf("message = %v, want %q", got.Message, "dependency unavailable")
	}
}

func TestPingCheck_Error_DoesNotLeakDetail(t *testing.T) {
	check := httpapi.PingCheck(&stubPinger{
		err: errors.New("failed to connect to host=postgres user=devopsbin database=devopsbin"),
	}, time.Second)

	got := check(context.Background())

	if got.Message == nil {
		t.Fatal("message = nil, want generic message")
	}
	for _, leak := range []string{"host=", "user=", "database=", "postgres"} {
		if strings.Contains(*got.Message, leak) {
			t.Fatalf("message %q leaked %q", *got.Message, leak)
		}
	}
}

func TestPingCheck_AppliesTimeout(t *testing.T) {
	p := &stubPinger{}
	check := httpapi.PingCheck(p, 50*time.Millisecond)

	check(context.Background())

	if _, ok := p.gotCtx.Deadline(); !ok {
		t.Fatal("expected ping context to carry a deadline")
	}
}

func TestPingCheck_NoTimeoutWhenZero(t *testing.T) {
	p := &stubPinger{}
	check := httpapi.PingCheck(p, 0)

	check(context.Background())

	if _, ok := p.gotCtx.Deadline(); ok {
		t.Fatal("expected ping context to have no deadline when timeout is zero")
	}
}
