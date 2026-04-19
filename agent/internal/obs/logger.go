package obs

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey string

const turnIDKey ctxKey = "turn_id"

var defaultLogger *slog.Logger

func init() {
	defaultLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

func WithTurnID(ctx context.Context, turnID string) context.Context {
	return context.WithValue(ctx, turnIDKey, turnID)
}

func TurnIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(turnIDKey).(string)
	return v
}

func Info(ctx context.Context, msg string, args ...any) {
	args = withTurnID(ctx, args)
	defaultLogger.InfoContext(ctx, msg, args...)
}

func Debug(ctx context.Context, msg string, args ...any) {
	args = withTurnID(ctx, args)
	defaultLogger.DebugContext(ctx, msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	args = withTurnID(ctx, args)
	defaultLogger.WarnContext(ctx, msg, args...)
}

func Error(ctx context.Context, msg string, args ...any) {
	args = withTurnID(ctx, args)
	defaultLogger.ErrorContext(ctx, msg, args...)
}

func withTurnID(ctx context.Context, args []any) []any {
	if id := TurnIDFrom(ctx); id != "" {
		return append([]any{"turn_id", id}, args...)
	}
	return args
}
