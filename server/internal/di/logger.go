package di

import (
	"go.uber.org/zap"
)

// ProvideLogger creates a production Zap logger.
func ProvideLogger() *zap.Logger {
	logger, err := zap.NewProduction()
	if err != nil {
		panic("di: failed to create logger: " + err.Error())
	}
	return logger
}
