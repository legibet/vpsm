package session

import (
	"context"
	"testing"
)

func TestIsUserInterruptErrorReturnsTrueForContextCanceled(t *testing.T) {
	t.Parallel()

	if !isUserInterruptError(context.Canceled) {
		t.Fatal("expected context.Canceled to be treated as user interrupt")
	}
}
