package logger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type GormLogger struct {
}

const TAG = "[GORM]"

func (l *GormLogger) LogMode(logger.LogLevel) logger.Interface {
	var sysLevel string
	switch logLevel {
	case DebugLevel:
		sysLevel = "debug"
	case InfoLevel:
		sysLevel = "info"
	case SevereLevel:
		sysLevel = "severe"
	case disableLevel:
		sysLevel = "disable"
	default:
		sysLevel = "unknown"
	}
	Infof("%s System Log Level is %s", TAG, sysLevel)
	return l
}

func (l *GormLogger) Info(ctx context.Context, str string, args ...interface{}) {
	WithContext(ctx).WithCallerSkip(2).Infof("%s Info: %s", TAG, str, args)
}

func (l *GormLogger) Warn(ctx context.Context, str string, args ...interface{}) {
	WithContext(ctx).WithCallerSkip(2).Infof("%s Warn: %s", TAG, str, args)
}

func (l *GormLogger) Error(ctx context.Context, str string, args ...interface{}) {
	WithContext(ctx).WithCallerSkip(2).Errorf("%s Error: %s", TAG, str, args)
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	sql, rowsAffected := fc()
	fields := []LogField{
		{
			Key:   "sql",
			Value: sql,
		},
		{
			Key:   "rows",
			Value: rowsAffected,
		},
	}
	if err != nil {
		fields = append(fields, LogField{
			Key:   "error",
			Value: err.Error(),
		})
		// A missed lookup is an expected outcome the caller handles (inbox
		// dedup probes, lazily-created rows, existence checks) — logging it
		// as an error drowns out real failures.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WithContext(ctx).WithCallerSkip(6).WithDuration(time.Since(begin)).Infow(TAG, fields...)
		} else {
			WithContext(ctx).WithCallerSkip(6).WithDuration(time.Since(begin)).Errorw(TAG, fields...)
		}
	} else {
		WithContext(ctx).WithCallerSkip(6).WithDuration(time.Since(begin)).Infow(fmt.Sprintf("%s SQL Executed", TAG), fields...)
	}
}
