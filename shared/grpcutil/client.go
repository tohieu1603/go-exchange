package grpcutil

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"time"

	"github.com/cryptox/shared/resilience"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// maxRecvMsgSize bounds inbound messages (4 MiB) to protect clients from
// oversized responses.
const maxRecvMsgSize = 4 * 1024 * 1024

// defaultCallTimeout bounds any single outbound RPC that does not already carry
// a deadline, so a hung dependency cannot block the caller's goroutine forever.
const defaultCallTimeout = 10 * time.Second

// defaultServiceConfig enables transparent client-side retries for transient
// failures (the dependency briefly unavailable or rate-limiting). Business
// errors (InvalidArgument, NotFound, …) are never retried. Combined with the
// circuit breaker below this gives "retry the blip, fail fast on the outage".
const defaultServiceConfig = `{
  "methodConfig": [{
    "name": [{}],
    "timeout": "15s",
    "retryPolicy": {
      "maxAttempts": 4,
      "initialBackoff": "0.1s",
      "maxBackoff": "2s",
      "backoffMultiplier": 2.0,
      "retryableStatusCodes": ["UNAVAILABLE", "RESOURCE_EXHAUSTED"]
    }
  }]
}`

// NewClient creates a gRPC client connection to addr with production-sane
// defaults: client-side keepalive pings, a bounded receive size, transport
// security selected by the GRPC_TLS env var, transparent retries for transient
// faults, and a per-connection circuit breaker (resilience.Breaker) that fails
// fast once the dependency is clearly down — preventing one sick downstream
// from cascading into the caller. It returns the error so callers (and tests)
// can decide how to handle a setup failure.
//
// Transport security:
//   - GRPC_TLS=true|1 → TLS using the host's root CA pool (for a TLS/mTLS
//     service mesh where inter-service traffic must be encrypted).
//   - otherwise       → insecure (plaintext) for the local/docker network.
func NewClient(addr string) (*grpc.ClientConn, error) {
	breaker := resilience.New(resilience.Config{MaxFailures: 5, OpenFor: 10 * time.Second})
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(transportCreds()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxRecvMsgSize)),
		grpc.WithDefaultServiceConfig(defaultServiceConfig),
		grpc.WithChainUnaryInterceptor(resilientUnaryInterceptor(breaker)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second, // ping idle conns every 30s
			Timeout:             10 * time.Second, // ...and drop them if no ack in 10s
			PermitWithoutStream: true,
		}),
	)
}

// Dial is the fail-fast variant of NewClient used by cmd/main.go wiring: a
// service that cannot construct a client to its dependency cannot start.
func Dial(addr string) *grpc.ClientConn {
	conn, err := NewClient(addr)
	if err != nil {
		log.Fatalf("gRPC dial %s failed: %v", addr, err)
	}
	return conn
}

// resilientUnaryInterceptor guards every outbound unary call with the circuit
// breaker and applies a default per-call timeout. Only transport-level faults
// (the dependency unreachable, timing out, or erroring internally) count
// against the breaker; ordinary business errors (InvalidArgument, NotFound, …)
// are treated as healthy responses so the breaker never trips on a valid "no".
func resilientUnaryInterceptor(b *resilience.Breaker) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if !b.Allow() {
			return status.Error(codes.Unavailable, "dependency unavailable (circuit open)")
		}
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultCallTimeout)
			defer cancel()
		}
		err := invoker(ctx, method, req, reply, cc, opts...)
		if isDownstreamFault(err) {
			b.Failure()
		} else {
			b.Success()
		}
		return err
	}
}

// isDownstreamFault reports whether err indicates the dependency itself is
// unhealthy (as opposed to returning a normal business error).
func isDownstreamFault(err error) bool {
	if err == nil {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Internal, codes.Unknown:
		return true
	default:
		return false
	}
}

func transportCreds() credentials.TransportCredentials {
	if v := os.Getenv("GRPC_TLS"); v == "true" || v == "1" {
		return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	return insecure.NewCredentials()
}
