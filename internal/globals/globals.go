/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

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
