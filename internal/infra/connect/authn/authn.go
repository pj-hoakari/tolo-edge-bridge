package authn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	connectrpc "connectrpc.com/connect"

	"github.com/pj-hoakari/tolo-edge-bridge/gen/tolo/edgebridge/v1/edgebridgev1connect"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/domain"
	"github.com/pj-hoakari/tolo-edge-bridge/internal/externaltoken"
)

const (
	authorizationHeader = "Authorization"
	bearerScheme        = "bearer"
)

var acceptedScopes = []string{"events.read", "events.report"}

var procedures = map[string][]domain.PeerRole{
	edgebridgev1connect.SignalingServiceRequestConnectionProcedure:   {domain.PeerRoleConsole},
	edgebridgev1connect.SignalingServiceIssueFirestoreTokenProcedure: {domain.PeerRoleEdge, domain.PeerRoleConsole},
}

var (
	ErrMissingVerifier = errors.New("authn: a verifier is required")
	ErrMissingClients  = errors.New("authn: at least one accepted client is required")

	errMissingAuthorization   = errors.New("authorization header is missing")
	errMalformedAuthorization = errors.New("authorization header is not one bearer token")
	errSenderConstrained      = errors.New("event_access carries a key binding")
	errUnknownClient          = errors.New("client_id is not an accepted client")
	errMissingScope           = errors.New("scope grants neither events.read nor events.report")
	errRoleNotAllowed         = errors.New("the role may not call this procedure")

	errUnauthenticated           = errors.New("unauthenticated")
	errPermissionDenied          = errors.New("permission denied")
	errAuthenticationUnavailable = errors.New("authentication unavailable")
)

type Principal struct {
	Subject   string
	ClientID  string
	Role      domain.PeerRole
	TenantID  string
	EventID   string
	ExpiresAt time.Time
}

type Verifier interface {
	Verify(ctx context.Context, token string) (externaltoken.Claims, error)
}

type Authenticator struct {
	verifier    Verifier
	clients     map[string]domain.PeerRole
	errorWriter *connectrpc.ErrorWriter
}

func New(verifier Verifier, clients map[string]domain.PeerRole) (*Authenticator, error) {
	if verifier == nil {
		return nil, ErrMissingVerifier
	}

	if len(clients) == 0 {
		return nil, ErrMissingClients
	}

	return &Authenticator{
		verifier:    verifier,
		clients:     clients,
		errorWriter: connectrpc.NewErrorWriter(),
	}, nil
}

func (a *Authenticator) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		principal, err := a.authenticate(ctx, r.Header, r.URL.Path)
		if err != nil {
			rejection := rejectionOf(err)

			slog.WarnContext(ctx, "rpc request rejected", "procedure", r.URL.Path, "code", rejection.Code().String(), "error", err)

			if writeErr := a.errorWriter.Write(w, r, rejection); writeErr != nil {
				slog.ErrorContext(ctx, "rpc error response write failed", "error", writeErr)
			}

			return
		}

		next.ServeHTTP(w, r.WithContext(NewContext(ctx, principal)))
	})
}

func (a *Authenticator) authenticate(ctx context.Context, header http.Header, procedure string) (Principal, error) {
	var zero Principal

	token, err := bearerToken(header)
	if err != nil {
		return zero, err
	}

	claims, err := a.verifier.Verify(ctx, token)
	if err != nil {
		return zero, err
	}

	if claims.Confirmation != "" {
		return zero, errSenderConstrained
	}

	role, accepted := a.clients[claims.ClientID]
	if !accepted {
		return zero, errUnknownClient
	}

	if !grantsAcceptedScope(claims.Scope) {
		return zero, errMissingScope
	}

	if !slices.Contains(procedures[procedure], role) {
		return zero, fmt.Errorf("%w: role %s on %s", errRoleNotAllowed, role, procedure)
	}

	return Principal{
		Subject:   claims.Subject,
		ClientID:  claims.ClientID,
		Role:      role,
		TenantID:  claims.TenantID,
		EventID:   claims.EventID,
		ExpiresAt: claims.ExpiresAt,
	}, nil
}

func grantsAcceptedScope(scope string) bool {
	granted := strings.Fields(scope)

	return slices.ContainsFunc(acceptedScopes, func(accepted string) bool { return slices.Contains(granted, accepted) })
}

func rejectionOf(err error) *connectrpc.Error {
	switch {
	case errors.Is(err, externaltoken.ErrKeysUnavailable):
		return connectrpc.NewError(connectrpc.CodeUnavailable, errAuthenticationUnavailable)
	case errors.Is(err, errUnknownClient), errors.Is(err, errMissingScope), errors.Is(err, errRoleNotAllowed):
		return connectrpc.NewError(connectrpc.CodePermissionDenied, errPermissionDenied)
	default:
		return connectrpc.NewError(connectrpc.CodeUnauthenticated, errUnauthenticated)
	}
}

func bearerToken(header http.Header) (string, error) {
	values, carried := header[http.CanonicalHeaderKey(authorizationHeader)]
	if !carried {
		return "", errMissingAuthorization
	}

	if len(values) != 1 {
		return "", errMalformedAuthorization
	}

	fields := strings.Fields(values[0])
	if len(fields) != 2 || !strings.EqualFold(fields[0], bearerScheme) {
		return "", errMalformedAuthorization
	}

	return fields[1], nil
}

type contextKey struct{}

func NewContext(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)

	return principal, ok
}
