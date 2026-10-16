package connect

import (
	"fmt"
	"net/http"

	connectrpc "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"connectrpc.com/otelconnect"

	"github.com/pj-hoakari/tolo-edge-bridge/gen/tolo/edgebridge/v1/edgebridgev1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/infra/connect/authn"
)

func Routes(signaling *application.SignalingService, authenticator *authn.Authenticator) (func(mux *http.ServeMux), error) {
	tracing, err := otelconnect.NewServerInterceptor()
	if err != nil {
		return nil, fmt.Errorf("create tracing interceptor: %w", err)
	}

	service := NewService(signaling)
	server := connectrpc.NewServer(tracing)
	edgebridgev1connect.RegisterSignalingServiceHandler(server, service)

	signalingMux := http.NewServeMux()
	connecthttp.Mount(signalingMux, server)

	return func(mux *http.ServeMux) {
		mux.Handle("/"+edgebridgev1connect.SignalingServiceName+"/", authenticator.Wrap(signalingMux))
	}, nil
}
