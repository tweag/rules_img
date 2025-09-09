package protohelper

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	credhelper "github.com/tweag/rules_img/src/pkg/auth/credential"
	"github.com/tweag/rules_img/src/pkg/auth/grpcheaderinterceptor"
)

func Client(uri string, helper credhelper.Helper, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts = slices.Clone(opts)
	schemeAndRest := strings.SplitN(uri, "://", 2)
	if len(schemeAndRest) != 2 {
		return nil, fmt.Errorf("invalid uri for grpc: %s", uri)
	}
	switch schemeAndRest[0] {
	case "grpc":
		// unencrypted grpc
		warnUnencryptedGRPC(uri)
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case "grpcs":
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	default:
		return nil, fmt.Errorf("unsupported scheme for grpc: %s", schemeAndRest[0])
	}

	target := fmt.Sprintf("dns:%s", schemeAndRest[1])

	opts = append(opts, grpcheaderinterceptor.DialOptions(helper)...)

	return grpc.NewClient(target, opts...)
}

func warnUnencryptedGRPC(uri string) {
	warnMutex.Lock()
	defer warnMutex.Unlock()

	if _, warned := WarnedURIs[uri]; warned {
		return
	}
	WarnedURIs[uri] = struct{}{}
	fmt.Fprintf(os.Stderr, "WARNING: using unencrypted grpc connection to %s - please consider using grpcs instead", uri)
}

// WarnedURIs is a set of URIs that have already been warned about.
// It is protected by warnMutex, which must be held when accessing it.
var (
	WarnedURIs = make(map[string]struct{})
	warnMutex  sync.Mutex
)
