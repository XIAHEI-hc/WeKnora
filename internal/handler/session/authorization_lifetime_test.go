package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBindAuthorizationRevocationCancelsAsyncWork(t *testing.T) {
	revoked := make(chan struct{})
	requestCtx := WithAuthorizationRevocation(context.Background(), revoked)
	asyncCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bindAuthorizationRevocation(requestCtx, cancel)
	close(revoked)

	select {
	case <-asyncCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("authorization revocation did not cancel async chat work")
	}
}

func TestBindAuthorizationRevocationLeavesOrdinaryRequestsAlone(t *testing.T) {
	asyncCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bindAuthorizationRevocation(context.Background(), cancel)
	require.NoError(t, asyncCtx.Err())
}
