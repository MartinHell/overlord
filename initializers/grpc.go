package initializers

import (
	"context"
	"os"

	"github.com/DCS-gRPC/go-bindings/dcs/v0/custom"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/mission"
	"github.com/DCS-gRPC/go-bindings/dcs/v0/net"
	"github.com/MartinHell/overlord/logs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var NetServiceClient net.NetServiceClient

var MissionServiceClient mission.MissionServiceClient

// CustomServiceClient reaches the mission scripting environment, used to poll
// the OVERLORD_EXPORT score table. Its Eval method only works when the DCS
// side sets evalEnabled = true.
var CustomServiceClient custom.CustomServiceClient

var GrpcClientConn *grpc.ClientConn

func InitGrpc() error {
	addr := os.Getenv("GRPC_SERVER_ADDRESS")
	if addr == "" {
		addr = "127.0.0.1:50051"
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	}

	// Lazy: this does not fail when DCS is down, it reconnects in the background
	// instead. That is what we want, since overlord is expected to outlive any
	// single DCS session.
	//
	// NewClient rather than the deprecated Dial. The two differ in more than a
	// name: Dial defaults to the passthrough resolver, NewClient to dns. For a
	// bare host:port that means a hostname is now actually resolved, and
	// re-resolved when the connection drops, so pointing GRPC_SERVER_ADDRESS at
	// a name whose address changes works instead of pinning to the first
	// answer. An IP literal behaves the same either way.
	var err error
	GrpcClientConn, err = grpc.NewClient(addr, opts...)
	if err != nil {
		return err
	}

	MissionServiceClient = mission.NewMissionServiceClient(GrpcClientConn)
	NetServiceClient = net.NewNetServiceClient(GrpcClientConn)
	CustomServiceClient = custom.NewCustomServiceClient(GrpcClientConn)

	logs.Sugar.Infof("gRPC client configured for %s", addr)

	return nil
}

// OpenEventStream opens a new StreamEvents stream. A gRPC stream cannot be
// reused once it has returned an error, so callers must open a fresh one on
// every reconnect rather than holding on to a single long-lived stream.
func OpenEventStream(ctx context.Context) (mission.MissionService_StreamEventsClient, error) {
	return MissionServiceClient.StreamEvents(ctx, &mission.StreamEventsRequest{})
}
