package connect

import (
	"fmt"
	"net/http"

	connectrpc "connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"github.com/pj-hoakari/tolo-edge-bridge/gen/signaling/v1/signalingv1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
)

func Routes(signaling *application.SignalingService) (func(mux *http.ServeMux), error) {
	tracing, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, fmt.Errorf("create tracing interceptor: %w", err)
	}

	service := NewService(signaling)
	interceptors := connectrpc.WithInterceptors(tracing)
	signalingPath, signalingHandler := signalingv1connect.NewSignalingServiceHandler(service, interceptors)

	return func(mux *http.ServeMux) {
		mux.Handle(signalingPath, signalingHandler)
	}, nil
}
