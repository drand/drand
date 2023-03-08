package context

import (
	"context"
	"testing"
)

type thisisatest bool

func TestSkipLogs(t *testing.T) {
	tests := []struct {
		name string
		args context.Context
		want bool
	}{
		{
			"set false",
			SetSkipLogs(context.Background(), false),
			false,
		},
		{
			"none set",
			context.Background(),
			false,
		},
		{
			"set true",
			SetSkipLogs(context.Background(), true),
			true,
		},
		{
			"set true twice",
			SetSkipLogs(SetSkipLogs(context.Background(), true), true),
			true,
		},
		{
			"set false twice",
			SetSkipLogs(SetSkipLogs(context.Background(), false), false),
			false,
		},
		{
			"set true set false",
			SetSkipLogs(SetSkipLogs(context.Background(), true), false),
			false,
		},
		{
			"set false set true",
			SetSkipLogs(SetSkipLogs(context.Background(), false), true),
			true,
		},
		{
			"set true and set other value",
			context.WithValue(SetSkipLogs(context.Background(), true), thisisatest(true), false),
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSkipLogsFromContext(tt.args); got != tt.want {
				t.Errorf("IsSkipLogsFromContext() = %v, want %v", got, tt.want)
			}
		})
	}
}
