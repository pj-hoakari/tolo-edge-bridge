package connect

import (
	"fmt"
	"net/http"

	connectrpc "connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"github.com/pj-hoakari/tolo-edge-bridge/gen/tolo/edgebridge/v1/edgebridgev1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/infra/connect/authn"
)

func Routes(signaling *application.SignalingService, authenticator *authn.Authenticator) (func(mux *http.ServeMux), error) {
	tracing, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, fmt.Errorf("create tracing interceptor: %w", err)
	}

	service := NewService(signaling)
	interceptors := connectrpc.WithInterceptors(tracing)
	signalingPath, signalingHandler := edgebridgev1connect.NewSignalingServiceHandler(service, interceptors)

	return func(mux *http.ServeMux) {
		mux.Handle(signalingPath, authenticator.Wrap(signalingHandler))
	}, nil
}
