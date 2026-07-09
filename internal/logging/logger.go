package logging

import (
	"context"

	"go.uber.org/zap"
)

type contextKey struct{}

func New() (*zap.Logger, error) {
	return zap.NewProduction()
}

func WithContext(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

func FromContext(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(contextKey{}).(*zap.Logger); ok && l != nil {
		return l
	}
	l, _ := zap.NewProduction()
	return l
}

func FieldRequestID(id string) zap.Field {
	return zap.String("request_id", id)
}
