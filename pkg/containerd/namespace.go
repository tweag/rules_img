package containerd

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// namespaceKey is the context key for the namespace
type namespaceKey struct{}

// WithNamespace returns a context with the specified namespace
func WithNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, namespaceKey{}, namespace)
}

// NamespaceFromContext gets the namespace from the context
func NamespaceFromContext(ctx context.Context) (string, bool) {
	namespace, ok := ctx.Value(namespaceKey{}).(string)
	return namespace, ok
}

// namespaceInterceptor adds namespace header to gRPC calls
func namespaceInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if ns, ok := NamespaceFromContext(ctx); ok {
		ctx = metadata.AppendToOutgoingContext(ctx, "containerd-namespace", ns)
	}
	return invoker(ctx, method, req, reply, cc, opts...)
}

// namespaceStreamInterceptor adds namespace header to streaming gRPC calls
func namespaceStreamInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if ns, ok := NamespaceFromContext(ctx); ok {
		ctx = metadata.AppendToOutgoingContext(ctx, "containerd-namespace", ns)
	}
	return streamer(ctx, desc, cc, method, opts...)
}
