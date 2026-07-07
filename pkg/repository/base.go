package repository

import (
	"context"

	"gorm.io/gorm"
)

// BaseRepository provides standard CRUD operations for any GORM model.
// T represents the GORM model struct.
type BaseRepository[T any] struct {
	DB *gorm.DB
}

// NewBaseRepository creates a new instance of BaseRepository.
func NewBaseRepository[T any](db *gorm.DB) *BaseRepository[T] {
	return &BaseRepository[T]{DB: db}
}

// Create inserts a new record into the database.
func (r *BaseRepository[T]) Create(ctx context.Context, entity *T) error {
	return r.DB.WithContext(ctx).Create(entity).Error
}

// FindByID retrieves a record by its UUID.
func (r *BaseRepository[T]) FindByID(ctx context.Context, id string) (*T, error) {
	var entity T
	err := r.DB.WithContext(ctx).Where("id = ?", id).First(&entity).Error
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// Update saves all fields of the entity.
func (r *BaseRepository[T]) Update(ctx context.Context, entity *T) error {
	return r.DB.WithContext(ctx).Save(entity).Error
}

// Delete performs a delete operation (soft-delete if gorm.DeletedAt is present).
func (r *BaseRepository[T]) Delete(ctx context.Context, id string) error {
	var entity T
	return r.DB.WithContext(ctx).Where("id = ?", id).Delete(&entity).Error
}
