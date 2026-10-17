// Package connect provides the Connect HTTP transport for this service.
package connect

import (
	"context"
	"errors"
	"log/slog"

	connectrpc "connectrpc.com/connect/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

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
		return connectrpc.NewError(connectrpc.CodeCanceled, "canceled").WithCause(err)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return connectrpc.NewError(connectrpc.CodeDeadlineExceeded, "deadline exceeded").WithCause(err)
	}

	slog.ErrorContext(ctx, "internal error", "error", err)

	return connectrpc.NewError(connectrpc.CodeInternal, "internal error").WithCause(errInternal) //nolint:forbidigo // the one place that builds internal errors
}

var clientErrors = []struct {
	target  error
	code    connectrpc.Code
	message string
}{
	{domain.ErrSubjectRequired, connectrpc.CodeUnauthenticated, "subject is missing from the token"},
	{domain.ErrTenantIDRequired, connectrpc.CodeUnauthenticated, "tenant_id is missing from the token"},
	{domain.ErrEventIDRequired, connectrpc.CodeUnauthenticated, "event_id is missing from the token"},
	{domain.ErrPeerRoleInvalid, connectrpc.CodeUnauthenticated, "client_id has no peer role"},
	{domain.ErrExpiresAtRequired, connectrpc.CodeUnauthenticated, "expiry is missing from the token"},
	{domain.ErrEdgeIDRequired, connectrpc.CodeInvalidArgument, "edge_device_id is required"},
}

func errorOf(ctx context.Context, err error) *connectrpc.Error {
	for _, e := range clientErrors {
		if errors.Is(err, e.target) {
			return connectrpc.NewError(e.code, e.message).WithCause(err)
		}
	}

	return InternalError(ctx, err)
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

func (s *Service) RequestConnection(ctx context.Context, req *edgebridgev1.RequestConnectionRequest) (*edgebridgev1.RequestConnectionResponse, error) {
	principal, _ := authn.FromContext(ctx)

	sessionID, err := s.signaling.RequestConnection(ctx, application.RequestConnectionInput{
		TenantID: principal.TenantID,
		EventID:  principal.EventID,
		EdgeID:   req.GetEdgeDeviceId(),
	})
	if err != nil {
		return nil, errorOf(ctx, err)
	}

	return &edgebridgev1.RequestConnectionResponse{SessionId: string(sessionID)}, nil
}

func (s *Service) IssueFirestoreToken(ctx context.Context, _ *edgebridgev1.IssueFirestoreTokenRequest) (*edgebridgev1.IssueFirestoreTokenResponse, error) {
	principal, _ := authn.FromContext(ctx)

	token, err := s.signaling.IssueFirestoreToken(ctx, application.IssueFirestoreTokenInput{
		Subject:   principal.Subject,
		TenantID:  principal.TenantID,
		EventID:   principal.EventID,
		Role:      principal.Role,
		ExpiresAt: principal.ExpiresAt,
	})
	if err != nil {
		return nil, errorOf(ctx, err)
	}

	return &edgebridgev1.IssueFirestoreTokenResponse{
		FirebaseCustomToken: token.Token,
		ExpiresAt:           timestamppb.New(token.ExpiresAt),
	}, nil
}
