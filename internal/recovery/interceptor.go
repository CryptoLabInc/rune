// Transport-layer gRPC interceptor
package recovery

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Retriggerer interface {
	Retrigger()
}

func UnaryRecovery(dep string, mgr Retriggerer) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err == nil || mgr == nil || !isRetryableGRPC(err) {
			return err
		}

		slog.Warn("recovery: retryable rpc failure - re-bootstrapping",
			"dep", dep,
			"method", method,
			"err", err,
		)
		mgr.Retrigger()
		return err
	}
}

func isRetryableGRPC(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
		return true
	}
	return false
}
