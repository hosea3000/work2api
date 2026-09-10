package server

import (
	"context"
	"github.com/hosea3000/work2api/internal/model"
	"github.com/hosea3000/work2api/pkg/log"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"os"
)

type MigrateServer struct {
	db  *gorm.DB
	log *log.Logger
}

func NewMigrateServer(db *gorm.DB, log *log.Logger) *MigrateServer {
	return &MigrateServer{
		db:  db,
		log: log,
	}
}
func (m *MigrateServer) Start(ctx context.Context) error {
	if err := m.db.AutoMigrate(
		&model.User{},
		&model.AdminUser{},
		&model.APIKey{},
		&model.CodeBuddyCredential{},
		&model.TraeCredential{},
		&model.CodeBuddyCheckinRecord{},
		&model.TraeCheckinRecord{},
		&model.PoolState{},
	); err != nil {
		m.log.Error("migrate error", zap.Error(err))
		return err
	}
	m.log.Info("AutoMigrate success")
	os.Exit(0)
	return nil
}
func (m *MigrateServer) Stop(ctx context.Context) error {
	m.log.Info("AutoMigrate stop")
	return nil
}
