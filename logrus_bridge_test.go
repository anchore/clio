package clio

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"

	upstreamLogrus "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/logrus"
)

// newCapturingLogger returns an application logger plus a hook recording every entry written through it.
func newCapturingLogger(t *testing.T) (logger.Logger, *logrustest.Hook) {
	t.Helper()
	backing := upstreamLogrus.New()
	l, err := logrus.Use(backing, logrus.Config{Level: logger.TraceLevel})
	require.NoError(t, err)
	return l, logrustest.NewLocal(backing)
}

// resetStdLogrus puts logrus.StandardLogger() back to its stock state now and when the test ends, since application
// setup bridges it process-wide.
func resetStdLogrus(t *testing.T) {
	t.Helper()
	reset := func() {
		std := upstreamLogrus.StandardLogger()
		std.ReplaceHooks(make(upstreamLogrus.LevelHooks))
		std.SetOutput(os.Stderr)
		std.SetFormatter(new(upstreamLogrus.TextFormatter))
		std.SetLevel(upstreamLogrus.InfoLevel)
		stdLogrusBridge = logrusBridge{}
	}
	reset()
	t.Cleanup(reset)
}

func TestLogrusBridge_forwardsEntries(t *testing.T) {
	dst, captured := newCapturingLogger(t)
	src := upstreamLogrus.New()

	var b logrusBridge
	b.attach(src, dst, logger.TraceLevel)

	src.WithField("errors", "xattr-err").Warn("lib-warn")
	src.Info("lib-info")
	src.Trace("lib-trace")
	src.WithField("from", "other").Error("lib-error")
	src.WithFields(upstreamLogrus.Fields{
		"response.status":            "401 Unauthorized",
		"response.header.set-cookie": "sid=secret",
		"Request.Header.Cookie":      "sid=secret",
		"body":                       "raw token endpoint body",
		"resp":                       &http.Response{Header: http.Header{"Set-Cookie": {"sid=secret"}}},
		"url":                        &url.URL{Scheme: "https", User: url.UserPassword("u", "secret"), Host: "registry"},
		"err":                        errors.New("bad\nfake=line"),
		"count":                      3,
	}).Debug("lib-response \x1b[31mred\nforged")

	entries := captured.AllEntries()
	require.Len(t, entries, 5)

	want := []struct {
		msg   string
		level upstreamLogrus.Level
	}{
		{"lib-warn", upstreamLogrus.WarnLevel},
		{"lib-info", upstreamLogrus.DebugLevel},
		{"lib-trace", upstreamLogrus.TraceLevel},
		{"lib-error", upstreamLogrus.ErrorLevel},
		{"lib-response  [31mred forged", upstreamLogrus.DebugLevel},
	}
	for i, w := range want {
		assert.Equal(t, w.msg, entries[i].Message)
		assert.Equal(t, w.level, entries[i].Level, w.msg)
		assert.Equal(t, bridgeVal, entries[i].Data[bridgeKey], w.msg)
	}
	assert.Equal(t, bridgeVal, entries[3].Data[bridgeKey], "bridge tag should win over an existing from field")

	assert.Equal(t, "xattr-err", entries[0].Data["errors"])

	resp := entries[4].Data
	assert.Equal(t, "401 Unauthorized", resp["response.status"])
	assert.NotContains(t, resp, "response.header.set-cookie")
	assert.NotContains(t, resp, "Request.Header.Cookie")
	assert.NotContains(t, resp, "body")
	assert.Equal(t, "<*http.Response>", resp["resp"])
	assert.Equal(t, "https://u:xxxxx@registry", resp["url"])
	assert.Equal(t, "bad fake=line", resp["err"])
	assert.Equal(t, 3, resp["count"])

	assert.Equal(t, io.Discard, src.Out)
}

func TestLogrusBridge_retargets(t *testing.T) {
	src := upstreamLogrus.New()
	first, firstCaptured := newCapturingLogger(t)
	second, secondCaptured := newCapturingLogger(t)

	var b logrusBridge
	b.attach(src, first, logger.WarnLevel)
	assert.Equal(t, upstreamLogrus.WarnLevel, src.GetLevel())

	b.attach(src, second, logger.DebugLevel)
	assert.Equal(t, upstreamLogrus.DebugLevel, src.GetLevel(), "a later attach should reset the level")
	assert.Len(t, src.Hooks[upstreamLogrus.WarnLevel], 1, "the hook should only be installed once")

	src.Warn("lib-warn")
	assert.Empty(t, firstCaptured.AllEntries())
	assert.Len(t, secondCaptured.AllEntries(), 1)
}

func TestLogrusBridge_noRecursionWhenSharingLogger(t *testing.T) {
	// the application logger is backed by the same logrus logger being bridged
	src := upstreamLogrus.New()
	dst, err := logrus.Use(src, logrus.Config{Level: logger.DebugLevel})
	require.NoError(t, err)
	captured := logrustest.NewLocal(src)

	var b logrusBridge
	b.attach(src, dst, logger.DebugLevel)

	src.Warn("lib-warn")

	// the original entry plus exactly one forwarded copy, then the guard stops it
	entries := captured.AllEntries()
	require.Len(t, entries, 2)
	assert.Equal(t, bridgeVal, entries[1].Data[bridgeKey])
}

func TestBridgeLevel(t *testing.T) {
	tests := []struct {
		level logger.Level
		want  upstreamLogrus.Level
	}{
		{logger.TraceLevel, upstreamLogrus.TraceLevel},
		{logger.DebugLevel, upstreamLogrus.DebugLevel},
		{logger.InfoLevel, upstreamLogrus.WarnLevel},
		{logger.WarnLevel, upstreamLogrus.WarnLevel},
		{logger.ErrorLevel, upstreamLogrus.ErrorLevel},
		{logger.DisabledLevel, upstreamLogrus.PanicLevel},
		{"WARNING", upstreamLogrus.WarnLevel},
		{"bogus", upstreamLogrus.WarnLevel},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			assert.Equal(t, tt.want, bridgeLevel(tt.level))
		})
	}
}

func TestLogrusBridge_concurrentAttach(t *testing.T) {
	src := upstreamLogrus.New()
	var b logrusBridge

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			src.Warn("lib-warn")
		}
	}()
	for range 100 {
		dst, _ := newCapturingLogger(t)
		b.attach(src, dst, logger.DebugLevel)
	}
	<-done

	assert.Len(t, src.Hooks[upstreamLogrus.WarnLevel], 1, "the hook should only be installed once")
}
