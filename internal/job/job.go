package job

import (
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/pkg/jwt"
	"github.com/yourname/work2api/pkg/log"
	"github.com/yourname/work2api/pkg/sid"
)

type Job struct {
	logger *log.Logger
	sid    *sid.Sid
	jwt    *jwt.JWT
	tm     repository.Transaction
}

func NewJob(
	tm repository.Transaction,
	logger *log.Logger,
	sid *sid.Sid,
) *Job {
	return &Job{
		logger: logger,
		sid:    sid,
		tm:     tm,
	}
}
