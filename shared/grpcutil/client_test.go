package grpcutil

import (
	"context"
	"testing"
	"time"

	"github.com/cryptox/shared/resilience"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewClient_LazyConnSucceeds(t *testing.T) {
	// grpc.NewClient is lazy — it validates the target and returns a conn
	// without dialing, so this must succeed without a live server.
	conn, err := NewClient("localhost:65000")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	assert.NotNil(t, conn)
}

func TestTransportCreds_SelectedByEnv(t *testing.T) {
	t.Setenv("GRPC_TLS", "")
	assert.Equal(t, "insecure", transportCreds().Info().SecurityProtocol,
		"default must be insecure (local/docker network)")

	t.Setenv("GRPC_TLS", "true")
	assert.Equal(t, "tls", transportCreds().Info().SecurityProtocol,
		"GRPC_TLS=true must select TLS credentials")
}

func TestIsDownstreamFault(t *testing.T) {
	assert.False(t, isDownstreamFault(nil))
	assert.False(t, isDownstreamFault(status.Error(codes.InvalidArgument, "x")), "business error is not a fault")
	assert.False(t, isDownstreamFault(status.Error(codes.NotFound, "x")), "business error is not a fault")
	assert.True(t, isDownstreamFault(status.Error(codes.Unavailable, "x")))
	assert.True(t, isDownstreamFault(status.Error(codes.DeadlineExceeded, "x")))
	assert.True(t, isDownstreamFault(status.Error(codes.Internal, "x")))
}

// fakeInvoker records how many times the underlying RPC was actually attempted.
func fakeInvoker(retErr error, attempts *int) grpc.UnaryInvoker {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		*attempts++
		return retErr
	}
}

func TestResilientInterceptor_OpensCircuitAfterFaults(t *testing.T) {
	b := resilience.New(resilience.Config{MaxFailures: 3})
	ic := resilientUnaryInterceptor(b)
	attempts := 0
	inv := fakeInvoker(status.Error(codes.Unavailable, "down"), &attempts)

	// 3 consecutive faults trip the breaker.
	for i := 0; i < 3; i++ {
		err := ic(context.Background(), "/svc/M", nil, nil, nil, inv)
		assert.Equal(t, codes.Unavailable, status.Code(err))
	}
	assert.Equal(t, 3, attempts)

	// Next call is rejected by the open breaker WITHOUT invoking the RPC.
	err := ic(context.Background(), "/svc/M", nil, nil, nil, inv)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, 3, attempts, "open circuit must short-circuit the call")
}

func TestResilientInterceptor_BusinessErrorsDoNotTripBreaker(t *testing.T) {
	b := resilience.New(resilience.Config{MaxFailures: 2})
	ic := resilientUnaryInterceptor(b)
	attempts := 0
	inv := fakeInvoker(status.Error(codes.NotFound, "missing"), &attempts)

	for i := 0; i < 5; i++ {
		err := ic(context.Background(), "/svc/M", nil, nil, nil, inv)
		assert.Equal(t, codes.NotFound, status.Code(err))
	}
	assert.Equal(t, 5, attempts, "NotFound is a healthy response; breaker must stay closed")
	assert.Equal(t, resilience.StateClosed, b.State())
}

func TestResilientInterceptor_InjectsDefaultDeadline(t *testing.T) {
	b := resilience.New(resilience.Config{})
	ic := resilientUnaryInterceptor(b)
	var hadDeadline bool
	inv := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		_, hadDeadline = ctx.Deadline()
		return nil
	}
	require.NoError(t, ic(context.Background(), "/svc/M", nil, nil, nil, inv))
	assert.True(t, hadDeadline, "a call without a deadline must get the default timeout")
}

func TestResilientInterceptor_RespectsCallerDeadline(t *testing.T) {
	b := resilience.New(resilience.Config{})
	ic := resilientUnaryInterceptor(b)
	deadlineCtx, dcancel := context.WithTimeout(context.Background(), time.Hour)
	defer dcancel()
	want, _ := deadlineCtx.Deadline()
	var got time.Time
	inv := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		got, _ = ctx.Deadline()
		return nil
	}
	require.NoError(t, ic(deadlineCtx, "/svc/M", nil, nil, nil, inv))
	assert.Equal(t, want, got, "caller's own deadline must be preserved, not overridden")
}
