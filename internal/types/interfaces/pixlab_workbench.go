package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// PixLabWorkbenchRepository owns the local half of the cross-system project binding.
type PixLabWorkbenchRepository interface {
	GetProjectBinding(ctx context.Context, projectCode string) (*types.PixLabProjectBinding, error)
}
