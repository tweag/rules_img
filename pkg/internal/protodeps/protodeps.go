// This package is used to force go.mod to keep
// dependencies of generated Go code.
package protodeps

import (
	_ "cloud.google.com/go/longrunning/autogen/longrunningpb"
	_ "google.golang.org/genproto/googleapis/api/annotations"
	_ "google.golang.org/genproto/googleapis/rpc/status"
	_ "google.golang.org/grpc"
	_ "google.golang.org/protobuf"
)
