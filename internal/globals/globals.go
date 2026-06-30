// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

package globals

import (
	"context"
	"log/slog"
	"os"
)

var (
	LogLevelMap = map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
)

type ApplicationContextOptions struct {
	LogLevel string
}

type ApplicationContext struct {
	Context context.Context
	Logger  *slog.Logger
}

func NewApplicationContext(opts ApplicationContextOptions) (*ApplicationContext, error) {

	logLevel, logLevelFound := LogLevelMap[opts.LogLevel]
	if !logLevelFound {
		logLevel = slog.LevelInfo
	}

	appCtx := &ApplicationContext{
		Context: context.Background(),
		Logger:  slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})),
	}

	//
	return appCtx, nil
}
