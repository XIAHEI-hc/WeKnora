package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

var ErrPixLabProjectBindingNotFound = errors.New("pixlab project binding not found")

type pixLabWorkbenchRepository struct {
	db *gorm.DB
}

func NewPixLabWorkbenchRepository(db *gorm.DB) interfaces.PixLabWorkbenchRepository {
	return &pixLabWorkbenchRepository{db: db}
}

func (r *pixLabWorkbenchRepository) GetProjectBinding(
	ctx context.Context,
	projectCode string,
) (*types.PixLabProjectBinding, error) {
	var binding types.PixLabProjectBinding
	if err := r.db.WithContext(ctx).
		Where("project_code = ?", projectCode).
		First(&binding).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPixLabProjectBindingNotFound
		}
		return nil, err
	}
	return &binding, nil
}
