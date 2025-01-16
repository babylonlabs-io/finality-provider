package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/babylonlabs-io/finality-provider/util"

	zaplogfmt "github.com/jsternberg/zap-logfmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewRootLogger(format string, level string, w io.Writer) (*zap.Logger, error) {
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = func(ts time.Time, encoder zapcore.PrimitiveArrayEncoder) {
		encoder.AppendString(ts.UTC().Format("2006-01-02T15:04:05.000000Z07:00"))
	}
	cfg.LevelKey = "lvl"

	var enc zapcore.Encoder
	switch format {
	case "json":
		enc = zapcore.NewJSONEncoder(cfg)
	case "auto", "console":
		enc = zapcore.NewConsoleEncoder(cfg)
	case "logfmt":
		enc = zaplogfmt.NewEncoder(cfg)
	default:
		return nil, fmt.Errorf("unrecognized log format %q", format)
	}

	var lvl zapcore.Level
	switch strings.ToLower(level) {
	case "panic":
		lvl = zap.PanicLevel
	case "fatal":
		lvl = zap.FatalLevel
	case "error":
		lvl = zap.ErrorLevel
	case "warn", "warning":
		lvl = zap.WarnLevel
	case "info":
		lvl = zap.InfoLevel
	case "debug":
		lvl = zap.DebugLevel
	default:
		return nil, fmt.Errorf("unsupported log level: %s", level)
	}

	return zap.New(
		zapcore.NewCore(
			enc,
			zapcore.AddSync(w),
			lvl,
		),
	), nil
}

func NewRootLoggerWithFile(logFile string, level string) (*zap.Logger, error) {
	if err := util.MakeDirectory(filepath.Dir(logFile)); err != nil {
		return nil, err
	}
	// #nosec G304 - The log file path is provided by the user and not externally
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	mw := io.MultiWriter(os.Stdout, f)

	logger, err := NewRootLogger("console", level, mw)
	if err != nil {
		return nil, err
	}

	return logger, nil
}
