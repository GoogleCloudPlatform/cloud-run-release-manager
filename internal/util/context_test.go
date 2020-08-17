package util_test

import (
	"context"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-run-release-manager/internal/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestLoggerFrom(t *testing.T) {
	tests := []struct {
		name   string
		logger *logrus.Logger
		level  logrus.Level
	}{
		{
			name:   "valid logger",
			logger: logrus.New(),
			level:  logrus.DebugLevel,
		},
		{
			name:   "nil logger",
			logger: nil,
			level:  logrus.InfoLevel,
		},
	}

	for _, test := range tests {
		var lg *logrus.Entry
		ctx := context.TODO()
		if test.logger != nil {
			test.logger.SetLevel(test.level)
			lg = logrus.NewEntry(test.logger)
			ctx = util.ContextWithLogger(ctx, lg)
		}

		returnedLg := util.LoggerFrom(ctx).Logger
		assert.Equal(t, test.level, returnedLg.Level)
	}
}
