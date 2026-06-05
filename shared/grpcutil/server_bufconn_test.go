package grpcutil

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/cryptox/shared/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// This is an end-to-end test of NewServer over a real gRPC server+client across
// an in-memory bufconn listener. It registers a hand-built service whose single
// method branches on the request string, exercising the full interceptor chain
// (recovery → timeout → error-map) the way production traffic would.

const bufMethod = "/grpcutil.test.Svc/Do"

func doHandler(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(wrapperspb.StringValue)
	if err := dec(in); err != nil {
		return nil, err
	}
	real := func(ctx context.Context, req any) (any, error) {
		switch req.(*wrapperspb.StringValue).Value {
		case "panic":
			panic("boom with secret token=topsecret")
		case "internal":
			return nil, apperr.Internal(errors.New("pq: password=hunter2"))
		case "invalid":
			return nil, apperr.Invalid("bad input")
		case "notfound":
			return nil, apperr.NotFound("missing")
		default:
			return wrapperspb.String("ok"), nil
		}
	}
	if interceptor == nil {
		return real(ctx, in)
	}
	return interceptor(ctx, in, &grpc.UnaryServerInfo{FullMethod: bufMethod}, real)
}

var bufServiceDesc = grpc.ServiceDesc{
	ServiceName: "grpcutil.test.Svc",
	HandlerType: (*any)(nil),
	Methods:     []grpc.MethodDesc{{MethodName: "Do", Handler: doHandler}},
	Metadata:    "test",
}

func startBufServer(t *testing.T) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := NewServer("buftest")
	srv.RegisterService(&bufServiceDesc, struct{}{})
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
		srv.GracefulStop()
	})
	return conn
}

func invoke(conn *grpc.ClientConn, cmd string) error {
	out := new(wrapperspb.StringValue)
	return conn.Invoke(context.Background(), bufMethod, wrapperspb.String(cmd), out)
}

func TestServer_E2E_OK(t *testing.T) {
	conn := startBufServer(t)
	out := new(wrapperspb.StringValue)
	err := conn.Invoke(context.Background(), bufMethod, wrapperspb.String("hello"), out)
	require.NoError(t, err)
	assert.Equal(t, "ok", out.Value)
}

func TestServer_E2E_PanicHiddenAsInternal(t *testing.T) {
	conn := startBufServer(t)
	err := invoke(conn, "panic")
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "internal error", st.Message())
	assert.NotContains(t, st.Message(), "topsecret", "panic detail must not cross the wire")
}

func TestServer_E2E_InternalErrorHidden(t *testing.T) {
	conn := startBufServer(t)
	err := invoke(conn, "internal")
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Equal(t, "internal error", st.Message())
	assert.NotContains(t, st.Message(), "hunter2")
}

func TestServer_E2E_ClassifiedErrorsMapped(t *testing.T) {
	conn := startBufServer(t)

	st, _ := status.FromError(invoke(conn, "invalid"))
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "bad input", st.Message(), "client-safe message is preserved")

	st2, _ := status.FromError(invoke(conn, "notfound"))
	assert.Equal(t, codes.NotFound, st2.Code())
	assert.Equal(t, "missing", st2.Message())
}
