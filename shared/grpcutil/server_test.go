package grpcutil

import (
	"context"
	"errors"
	"testing"

	"github.com/cryptox/shared/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func unaryInfo(method string) *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{FullMethod: method}
}

func TestRecoveryUnaryInterceptor_ConvertsPanicToInternal(t *testing.T) {
	ic := recoveryUnaryInterceptor("test")
	resp, err := ic(context.Background(), nil, unaryInfo("/svc/M"), func(context.Context, any) (any, error) {
		panic("kaboom")
	})
	assert.Nil(t, resp)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "internal error", st.Message(), "panic detail must not leak")
}

func TestRecoveryUnaryInterceptor_PassesThroughNormalReturn(t *testing.T) {
	ic := recoveryUnaryInterceptor("test")
	resp, err := ic(context.Background(), "in", unaryInfo("/svc/M"), func(_ context.Context, req any) (any, error) {
		return "out", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "out", resp)
}

func TestTimeoutUnaryInterceptor_InjectsDeadline(t *testing.T) {
	ic := timeoutUnaryInterceptor(defaultHandlerTimeout)
	var hadDeadline bool
	_, err := ic(context.Background(), nil, unaryInfo("/svc/M"), func(ctx context.Context, _ any) (any, error) {
		_, hadDeadline = ctx.Deadline()
		return nil, nil
	})
	assert.NoError(t, err)
	assert.True(t, hadDeadline, "handler context must carry a deadline")
}

func TestErrorMapUnaryInterceptor_HidesInternalDetail(t *testing.T) {
	ic := errorMapUnaryInterceptor()
	_, err := ic(context.Background(), nil, unaryInfo("/svc/M"), func(context.Context, any) (any, error) {
		return nil, apperr.Internal(errors.New("pq: connection refused at 10.0.0.5"))
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "internal error", st.Message(), "DB detail must never reach the wire")
	assert.NotContains(t, st.Message(), "pq:")
}

func TestErrorMapUnaryInterceptor_MapsClassifiedKinds(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code codes.Code
		msg  string
	}{
		{"invalid", apperr.Invalid("bad amount"), codes.InvalidArgument, "bad amount"},
		{"notfound", apperr.NotFound("wallet missing"), codes.NotFound, "wallet missing"},
		{"conflict", apperr.Conflict("already exists"), codes.AlreadyExists, "already exists"},
		{"forbidden", apperr.Forbidden("nope"), codes.PermissionDenied, "nope"},
	}
	ic := errorMapUnaryInterceptor()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ic(context.Background(), nil, unaryInfo("/svc/M"), func(context.Context, any) (any, error) {
				return nil, tc.err
			})
			st, _ := status.FromError(err)
			assert.Equal(t, tc.code, st.Code())
			assert.Equal(t, tc.msg, st.Message(), "classified messages are safe to expose")
		})
	}
}

func TestErrorMapUnaryInterceptor_PassesThroughDownstreamStatus(t *testing.T) {
	ic := errorMapUnaryInterceptor()
	down := status.Error(codes.Unavailable, "downstream down")
	_, err := ic(context.Background(), nil, unaryInfo("/svc/M"), func(context.Context, any) (any, error) {
		return nil, down
	})
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code(), "an existing downstream status is preserved")
}

func TestErrorMapUnaryInterceptor_NilStaysNil(t *testing.T) {
	ic := errorMapUnaryInterceptor()
	resp, err := ic(context.Background(), nil, unaryInfo("/svc/M"), func(context.Context, any) (any, error) {
		return "ok", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestNewServer_ConstructsAndStops(t *testing.T) {
	srv := NewServer("test")
	require.NotNil(t, srv)
	srv.GracefulStop() // no listener; must return promptly
}
