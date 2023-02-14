package context

import (
	ccontext "context"
)

type skipType bool

var skipLogging skipType = true

func SetSkipLogs(ctx ccontext.Context, value bool) ccontext.Context {
	return ccontext.WithValue(ctx, skipLogging, value)
}

func IsSkipLogsFromContext(ctx ccontext.Context) bool {
	value, ok := ctx.Value(skipLogging).(bool)
	if ok {
		return value
	}
	return false
}
