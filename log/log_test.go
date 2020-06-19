package log

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"

	"github.com/go-kit/kit/log"
	lvl "github.com/go-kit/kit/log/level"
	"github.com/stretchr/testify/require"
)

func TestLoggerKit(t *testing.T) {
	const (
		Info int = iota
		Debug
		Warn
		Error
	)

	type logTest struct {
		with  []interface{}
		opts  lvl.Option
		level int
		msg   string
		out   []string
	}

	w := func(kv ...interface{}) []interface{} {
		return kv
	}
	o := func(outs ...string) []string {
		return outs
	}
	var tests = []logTest{
		{nil, nil, Info, "hello", o("hello")},
		{nil, lvl.AllowInfo(), Debug, "hello", nil},
		{w("yard", "bird"), lvl.AllowWarn(), Warn, "hello", o("yard", "bird", "hello")},
	}

	for i, test := range tests {
		t.Logf(" -- test %d -- ", i)
		var b bytes.Buffer
		logger := log.NewLogfmtLogger(&b)

		if test.opts != nil {
			logger = lvl.NewFilter(logger, test.opts)
		}
		kit := NewKitLoggerFrom(logger)
		if test.with != nil {
			kit = kit.With(test.with...)
		}
		var logging func(...interface{})
		switch test.level {
		case Info:
			logging = kit.Info
		case Debug:
			logging = kit.Debug
		case Warn:
			logging = kit.Warn
		case Error:
			logging = kit.Error
		default:
			t.FailNow()
		}

		logging("msg", test.msg)
		if test.out != nil {
			requireContains(t, &b, test.out, true)
		} else {
			requireContains(t, &b, nil, false)
		}
	}
}

func requireContains(t *testing.T, r io.Reader, outs []string, present bool) {
	out, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	if !present {
		require.Equal(t, string(out), "")
		return
	}
	for _, o := range outs {
		require.Contains(t, string(out), o)
	}
}
