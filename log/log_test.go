package log

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestLoggerKit(t *testing.T) {
	type logTest struct {
		with       []interface{}
		level      int
		allowedLvl int
		msg        string
		out        []string
	}

	w := func(kv ...interface{}) []interface{} {
		return kv
	}
	o := func(outs ...string) []string {
		return outs
	}
	var tests = []logTest{
		{nil, LogInfo, LogInfo, "hello", o("hello")},
		{nil, LogDebug, LogInfo, "hello", nil},
		{nil, LogError, LogDebug, "hello", o("hello")},
		{nil, LogWarn, LogError, "hello", nil},
		{nil, LogWarn, LogDebug, "hello", o("hello")},
		{w("yard", "bird"), LogWarn, LogInfo, "hello", o("yard", "bird", "hello")},
	}

	for i, test := range tests {
		t.Logf(" -- test %d -- ", i)

		var b bytes.Buffer
		writer := bufio.NewWriter(&b)
		syncer := zapcore.AddSync(writer)

		var logging func(...interface{})
		logger := NewLogger(syncer, test.allowedLvl)

		if test.with != nil {
			logger = logger.With(test.with...)
		}

		switch test.level {
		case LogInfo:
			logging = logger.Info
		case LogDebug:
			logging = logger.Debug
		case LogWarn:
			logging = logger.Warn
		case LogError:
			logging = logger.Error
		case LogFatal:
			logging = logger.Fatal
		case LogPanic:
			logging = logger.Panic
		default:
			t.FailNow()
		}

		logging("msg=", test.msg)
		writer.Flush()

		if test.out != nil {
			requireContains(t, &b, test.out, true)
		} else {
			requireContains(t, &b, nil, false)
		}
	}
}

func TestOddKV(t *testing.T) {
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	syncer := zapcore.AddSync(writer)

	logger := NewLogger(syncer, LogInfo)
	logger = logger.With([]interface{}{"yard", "bird", "stone"}...)

	logger.Info("msg=", "hello")
	writer.Flush()

	out := b.String()

	require.Contains(t, out, "msg=hello")
	require.Contains(t, out, "Ignored key without a value.")
}

func requireContains(t *testing.T, r io.Reader, outs []string, present bool) {
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	if !present {
		require.Equal(t, string(out), "")
		return
	}
	for _, o := range outs {
		require.Contains(t, string(out), o)
	}
	require.NotContains(t, string(out), "Ignored key without a value.")
}
