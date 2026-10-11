// Package connect provides the Connect HTTP transport for this service.
package connect

import (
	"fmt"
	"net/http"

	connectrpc "connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"github.com/pj-hoakari/tolo-edge-bridge/gen/greet/v1/greetv1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
)

func Routes(greetService application.GreetUseCases) (func(mux *http.ServeMux), error) {
	tracing, err := otelconnect.NewInterceptor()
	if err != nil {
		return nil, fmt.Errorf("create tracing interceptor: %w", err)
	}

	path, handler := greetv1connect.NewGreetServiceHandler(
		NewService(greetService),
		connectrpc.WithInterceptors(tracing),
	)

	return func(mux *http.ServeMux) {
		mux.Handle(path, handler)
	}, nil
}
