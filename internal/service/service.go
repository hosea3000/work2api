package service

import (
	"github.com/hosea3000/work2api/internal/repository"
	"github.com/hosea3000/work2api/pkg/jwt"
	"github.com/hosea3000/work2api/pkg/log"
	"github.com/hosea3000/work2api/pkg/sid"
)

type Service struct {
	logger *log.Logger
	sid    *sid.Sid
	jwt    *jwt.JWT
	tm     repository.Transaction
}

func NewService(
	tm repository.Transaction,
	logger *log.Logger,
	sid *sid.Sid,
	jwt *jwt.JWT,
) *Service {
	return &Service{
		logger: logger,
		sid:    sid,
		jwt:    jwt,
		tm:     tm,
	}
}
