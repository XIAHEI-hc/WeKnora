package session

import "context"

type authorizationRevocationContextKey struct{}

// WithAuthorizationRevocation attaches a host-managed authorization lifetime
// to a chat request. Ordinary WeKnora requests do not set this value.
func WithAuthorizationRevocation(ctx context.Context, revoked <-chan struct{}) context.Context {
	if revoked == nil {
		return ctx
	}
	return context.WithValue(ctx, authorizationRevocationContextKey{}, revoked)
}

func bindAuthorizationRevocation(ctx context.Context, cancel context.CancelFunc) {
	revoked, _ := ctx.Value(authorizationRevocationContextKey{}).(<-chan struct{})
	if revoked == nil {
		return
	}
	go func() {
		select {
		case <-revoked:
			cancel()
		case <-ctx.Done():
		}
	}()
}
