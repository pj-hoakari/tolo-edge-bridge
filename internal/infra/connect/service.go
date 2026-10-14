// Package connect provides the Connect HTTP transport for this service.
package connect

import (
	"context"
	"errors"
	"log/slog"

	connectrpc "connectrpc.com/connect"

	edgebridgev1 "github.com/pj-hoakari/tolo-edge-bridge/gen/tolo/edgebridge/v1"
	"github.com/pj-hoakari/tolo-edge-bridge/gen/tolo/edgebridge/v1/edgebridgev1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/infra/connect/authn"
)

// errInternal is the only detail a client learns about an internal failure.
var errInternal = errors.New("internal error")

// InternalError reports a failure the client can do nothing about. The cause is
// written to the server log and replaced by a fixed message, so that no
// internal detail leaves the service. The log handler names the trace of the
// request context on the record, so an operator can find the failure in the
// trace it belongs to.
//
// A cancelled or timed-out request is the client going away rather than a
// server fault, so it keeps its own code and is not logged.
//
// It is exported for the other transports of this process, so that every
// service answers an internal failure the same way.
func InternalError(ctx context.Context, err error) *connectrpc.Error {
	if errors.Is(err, context.Canceled) {
		return connectrpc.NewError(connectrpc.CodeCanceled, err)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return connectrpc.NewError(connectrpc.CodeDeadlineExceeded, err)
	}

	slog.ErrorContext(ctx, "internal error", "error", err)

	return connectrpc.NewError(connectrpc.CodeInternal, errInternal) //nolint:forbidigo // the one place that builds internal errors
}

type Service struct {
	edgebridgev1connect.UnimplementedSignalingServiceHandler
	signaling *application.SignalingService
}

func NewService(signaling *application.SignalingService) *Service {
	return &Service{
		UnimplementedSignalingServiceHandler: edgebridgev1connect.UnimplementedSignalingServiceHandler{},
		signaling:                            signaling,
	}
}

func (s *Service) RequestConnection(ctx context.Context, req *connectrpc.Request[edgebridgev1.RequestConnectionRequest]) (*connectrpc.Response[edgebridgev1.RequestConnectionResponse], error) {
	principal, _ := authn.FromContext(ctx)

	sessionID, err := s.signaling.RequestConnection(ctx, application.RequestConnectionInput{
		TenantID: principal.TenantID,
		EventID:  principal.EventID,
		EdgeID:   req.Msg.GetEdgeDeviceId(),
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrTenantIDRequired), errors.Is(err, domain.ErrEventIDRequired):
			return nil, connectrpc.NewError(connectrpc.CodeUnauthenticated, err)
		case errors.Is(err, domain.ErrEdgeIDRequired):
			return nil, connectrpc.NewError(connectrpc.CodeInvalidArgument, err)
		default:
			return nil, InternalError(ctx, err)
		}
	}

	return connectrpc.NewResponse(&edgebridgev1.RequestConnectionResponse{SessionId: string(sessionID)}), nil
}
