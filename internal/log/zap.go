package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.SugaredLogger

func Setup(dev bool, debug bool) {
	var logger *zap.Logger

	if dev {
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		logger, _ = config.Build()
	} else {
		cfg := zap.NewProductionConfig()
		if debug {
			cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		}
		logger, _ = cfg.Build()
	}
	defer logger.Sync() // nolint:errcheck
	Log = logger.Sugar()
}
