// Package connect provides the Connect HTTP transport for this service.
package connect

import (
	"context"
	"errors"
	"log/slog"

	connectrpc "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	signalingv1 "github.com/pj-hoakari/tolo-edge-bridge/gen/signaling/v1"
	"github.com/pj-hoakari/tolo-edge-bridge/gen/signaling/v1/signalingv1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/application"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
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
	signalingv1connect.UnimplementedEdgeRegistryServiceHandler
	signalingv1connect.UnimplementedSignalingServiceHandler
	signaling *application.SignalingService
}

func NewService(signaling *application.SignalingService) *Service {
	return &Service{
		UnimplementedEdgeRegistryServiceHandler: signalingv1connect.UnimplementedEdgeRegistryServiceHandler{},
		UnimplementedSignalingServiceHandler:    signalingv1connect.UnimplementedSignalingServiceHandler{},
		signaling:                               signaling,
	}
}

func (s *Service) ListAliveEdges(ctx context.Context, _ *connectrpc.Request[signalingv1.ListAliveEdgesRequest]) (*connectrpc.Response[signalingv1.ListAliveEdgesResponse], error) {
	edges, err := s.signaling.ListAliveEdges(ctx)
	if err != nil {
		return nil, InternalError(ctx, err)
	}

	res := make([]*signalingv1.Edge, 0, len(edges))
	for _, edge := range edges {
		res = append(res, &signalingv1.Edge{
			Id:         string(edge.ID),
			LastSeenAt: timestamppb.New(edge.LastSeenAt),
		})
	}

	return connectrpc.NewResponse(&signalingv1.ListAliveEdgesResponse{Edges: res}), nil
}

func (s *Service) RequestConnection(ctx context.Context, req *connectrpc.Request[signalingv1.RequestConnectionRequest]) (*connectrpc.Response[signalingv1.RequestConnectionResponse], error) {
	sessionID, err := s.signaling.RequestConnection(ctx, application.RequestConnectionInput{
		EdgeID: req.Msg.GetEdgeId(),
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrEdgeIDRequired):
			return nil, connectrpc.NewError(connectrpc.CodeInvalidArgument, err)
		case errors.Is(err, domain.ErrEdgeNotFound):
			return nil, connectrpc.NewError(connectrpc.CodeNotFound, err)
		default:
			return nil, InternalError(ctx, err)
		}
	}

	return connectrpc.NewResponse(&signalingv1.RequestConnectionResponse{SessionId: string(sessionID)}), nil
}
