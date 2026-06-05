package grpcutil

import (
	"context"
	"log"
	"runtime/debug"
	"time"

	"github.com/cryptox/shared/grpcerr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// defaultHandlerTimeout bounds how long any unary RPC handler may run before its
// context is cancelled, so one stuck call cannot pin a server goroutine forever.
const defaultHandlerTimeout = 15 * time.Second

// NewServer builds a *grpc.Server hardened with the interceptors every service
// needs, so no service re-implements them (and none can forget one):
//
//   - panic recovery: a panicking handler returns codes.Internal "internal
//     error" instead of crashing the process or leaking a stack trace.
//   - error mapping: handler errors pass through grpcerr.ToStatus, so apperr
//     Kinds become the right gRPC code and KindInternal detail is hidden.
//   - timeout: each unary handler gets a bounded context.
//   - keepalive: enforce client ping policy and bound idle/aged connections.
//
// name labels the server in recovery logs. Extra options (e.g. registering a
// TLS credential or stats handler) may be appended via opts.
func NewServer(name string, opts ...grpc.ServerOption) *grpc.Server {
	base := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxRecvMsgSize),
		grpc.ChainUnaryInterceptor(
			recoveryUnaryInterceptor(name),
			timeoutUnaryInterceptor(defaultHandlerTimeout),
			errorMapUnaryInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			recoveryStreamInterceptor(name),
		),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			Time:              30 * time.Second,
			Timeout:           10 * time.Second,
		}),
	}
	return grpc.NewServer(append(base, opts...)...)
}

// recoveryUnaryInterceptor converts a handler panic into a safe Internal error.
func recoveryUnaryInterceptor(name string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[%s][grpc] panic in %s: %v\n%s", name, info.FullMethod, r, debug.Stack())
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

// recoveryStreamInterceptor is the streaming counterpart of the unary recovery.
func recoveryStreamInterceptor(name string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[%s][grpc] panic in %s: %v\n%s", name, info.FullMethod, r, debug.Stack())
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(srv, ss)
	}
}

// timeoutUnaryInterceptor bounds each unary handler's context.
func timeoutUnaryInterceptor(d time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return handler(ctx, req)
	}
}

// errorMapUnaryInterceptor runs after the handler and maps any returned error
// through grpcerr.ToStatus, so handlers may return raw apperr/domain errors and
// internal detail is never exposed on the wire. grpcerr.ToStatus already passes
// a downstream gRPC status through unchanged and hides KindInternal detail.
func errorMapUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		return resp, grpcerr.ToStatus(err)
	}
}
