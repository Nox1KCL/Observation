// Package logger provides logging utilities for the InFolderSort application.
package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"gopkg.in/natefinch/lumberjack.v2"
)

var llog = slog.With("module", "logger")

type LumberConfig struct {
	MaxSize    int  `toml:"max_size"`
	MaxAge     int  `toml:"max_age"`
	MaxBackups int  `toml:"max_backups"`
	Compress   bool `toml:"compress"`
}

type LeveledHandler struct {
	handlers []handlerEntry
}

type handlerEntry struct {
	level   slog.Level
	handler slog.Handler
}

func (h *LeveledHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, entry := range h.handlers {
		if entry.handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *LeveledHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, entry := range h.handlers {
		if entry.handler.Enabled(ctx, r.Level) {
			if err := entry.handler.Handle(ctx, r); err != nil {
				errs = append(errs, err)
			}
		}
	}
	err := errors.Join(errs...)
	return err
}

func (h *LeveledHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var newHandles []handlerEntry
	for _, entry := range h.handlers {
		newHandles = append(newHandles, handlerEntry{entry.level, entry.handler.WithAttrs(attrs)})
	}
	return &LeveledHandler{handlers: newHandles}
}
func (h *LeveledHandler) WithGroup(name string) slog.Handler {
	var newHandles []handlerEntry
	for _, entry := range h.handlers {
		newHandles = append(newHandles, handlerEntry{entry.level, entry.handler.WithGroup(name)})
	}
	return &LeveledHandler{handlers: newHandles}
}

func GetHandler(cfg *LumberConfig, levels map[slog.Level]string, otelLoggerName string) (*LeveledHandler, error) {
	var handlers []handlerEntry
	
	for level, path := range levels {
		handler, err := HandlerConveyor(path, level, cfg)
		if err != nil {
			return nil, fmt.Errorf("creating logger %q: %w", path, err)
		}
		handlers = append(handlers, handlerEntry{level, handler})
	}

	if otelLoggerName != "" {
		otelHandler := otelslog.NewHandler(otelLoggerName)
		handlers = append(handlers, handlerEntry{slog.LevelDebug, otelHandler})
	}

	logHandler := &LeveledHandler{handlers}

	llog.Info("logger handlers created successfully", 
		"file_handlers_count", len(levels),
		"otel_enabled", otelLoggerName != "",
	)
	return logHandler, nil
}

func HandlerConveyor(filename string, level slog.Level, cfg *LumberConfig) (slog.Handler, error) {
	absInfoPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("getting abs path %q: %w", absInfoPath, err)
	}
	logger := &lumberjack.Logger{
		Filename:   absInfoPath,
		MaxSize:    cfg.MaxSize,
		MaxAge:     cfg.MaxAge,
		MaxBackups: cfg.MaxBackups,
		Compress:   cfg.Compress,
	}
	handler := slog.NewJSONHandler(logger, &slog.HandlerOptions{Level: level})
	return handler, nil
}
