package repo

import (
	"context"
	"github.com/perfect-panel/server/internal/repository"

	"github.com/perfect-panel/server/internal/module/platform/entity/client"
	"gorm.io/gorm"
)

var _ repository.ClientRepo = (*clientRepo)(nil)

type clientRepo struct {
	*gorm.DB
}

// NewClientRepo builds the module-owned implementation.
func NewClientRepo(db *gorm.DB) repository.ClientRepo {
	return &clientRepo{
		DB: db,
	}
}

func (m *clientRepo) Insert(ctx context.Context, data *client.SubscribeApplication) error {
	if err := m.WithContext(ctx).Model(&client.SubscribeApplication{}).Create(data).Error; err != nil {
		return err
	}
	return nil
}

func (m *clientRepo) FindOne(ctx context.Context, id int64) (*client.SubscribeApplication, error) {
	var resp client.SubscribeApplication
	if err := m.WithContext(ctx).Model(&client.SubscribeApplication{}).Where("id = ?", id).First(&resp).Error; err != nil {
		return nil, err
	}
	return &resp, nil
}

func (m *clientRepo) Update(ctx context.Context, data *client.SubscribeApplication) error {
	if _, err := m.FindOne(ctx, data.Id); err != nil {
		return err
	}
	if err := m.WithContext(ctx).Model(&client.SubscribeApplication{}).Where("id = ?", data.Id).Save(data).Error; err != nil {
		return err
	}
	return nil
}

func (m *clientRepo) Delete(ctx context.Context, id int64) error {
	if err := m.WithContext(ctx).Model(&client.SubscribeApplication{}).Where("id = ?", id).Delete(&client.SubscribeApplication{}).Error; err != nil {
		return err
	}
	return nil
}

func (m *clientRepo) List(ctx context.Context) ([]*client.SubscribeApplication, error) {
	var resp []*client.SubscribeApplication
	if err := m.WithContext(ctx).Find(&resp).Error; err != nil {
		return nil, err
	}
	return resp, nil
}
