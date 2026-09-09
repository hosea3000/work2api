package task

import (
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/pkg/jwt"
	"github.com/yourname/work2api/pkg/log"
	"github.com/yourname/work2api/pkg/sid"
)

type Task struct {
	logger *log.Logger
	sid    *sid.Sid
	jwt    *jwt.JWT
	tm     repository.Transaction
}

func NewTask(
	tm repository.Transaction,
	logger *log.Logger,
	sid *sid.Sid,
) *Task {
	return &Task{
		logger: logger,
		sid:    sid,
		tm:     tm,
	}
}
