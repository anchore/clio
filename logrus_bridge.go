package clio

import (
	"io"
	"strings"
	"sync"
	"sync/atomic"

	upstreamLogrus "github.com/sirupsen/logrus"

	"github.com/anchore/go-logger"
)

const (
	bridgeKey = "from"
	bridgeVal = "logrus"
)

// stdLogrusBridge routes the process-global logrus logger into the application logger. Libraries such as containerd
// and the containers/podman libraries log through logrus.StandardLogger(), which nothing configures, so without this
// their output prints raw to stderr underneath the application's logger or UI.
var stdLogrusBridge logrusBridge

type logrusBridge struct {
	once   sync.Once
	target atomic.Pointer[logger.Logger]
}

// attach takes over src (once) and forwards its entries to dst. Repeated calls retarget the bridge and reset the
// level. src's own output is discarded, so this must not be used on a logger the application writes through directly.
func (b *logrusBridge) attach(src *upstreamLogrus.Logger, dst logger.Logger, level logger.Level) {
	b.target.Store(&dst)
	b.once.Do(func() {
		// add the hook before discarding output so no entries are lost in between
		src.AddHook(b)
		// formatting entries that are written to io.Discard is wasted work
		src.SetFormatter(nopFormatter{})
		src.SetOutput(io.Discard)
	})
	src.SetLevel(bridgeLevel(level))
}

// bridgeLevel picks the most verbose logrus level whose entries would still be visible at the given level, keeping in
// mind that Fire demotes info entries to debug. This is only a pre-filter (the target logger filters again), so an
// unrecognized level fails open rather than dropping library errors.
func bridgeLevel(level logger.Level) upstreamLogrus.Level {
	lvl, err := logger.LevelFromString(string(level))
	if err != nil {
		return upstreamLogrus.WarnLevel
	}
	switch lvl {
	case logger.TraceLevel:
		return upstreamLogrus.TraceLevel
	case logger.DebugLevel:
		return upstreamLogrus.DebugLevel
	case logger.InfoLevel, logger.WarnLevel:
		return upstreamLogrus.WarnLevel
	case logger.ErrorLevel:
		return upstreamLogrus.ErrorLevel
	}
	// disabled
	return upstreamLogrus.PanicLevel
}

func (b *logrusBridge) Levels() []upstreamLogrus.Level {
	return upstreamLogrus.AllLevels
}

func (b *logrusBridge) Fire(entry *upstreamLogrus.Entry) error {
	dst := b.target.Load()
	// guard against recursion when the target is itself backed by the bridged logger: our own forwarded entry would
	// come back through this hook carrying the bridge field
	if dst == nil || entry.Data[bridgeKey] == bridgeVal {
		return nil
	}

	fields := make([]any, 0, 2*len(entry.Data)+2)
	for k, v := range entry.Data {
		if dropBridgedField(k) {
			continue
		}
		fields = append(fields, k, v)
	}
	// set last so it wins over any existing "from" field
	fields = append(fields, bridgeKey, bridgeVal)

	l := (*dst).Nested(fields...)
	switch entry.Level {
	case upstreamLogrus.PanicLevel, upstreamLogrus.FatalLevel, upstreamLogrus.ErrorLevel:
		l.Error(entry.Message)
	case upstreamLogrus.WarnLevel:
		l.Warn(entry.Message)
	case upstreamLogrus.TraceLevel:
		l.Trace(entry.Message)
	default:
		// libraries tend to be chatty at info, which is debug-level detail from the application's point of view
		l.Debug(entry.Message)
	}
	return nil
}

// dropBridgedField filters fields that can carry secrets the redactor has no way of knowing about, such as
// containerd's per-request HTTP headers (e.g. a registry session cookie in "response.header.set-cookie") and raw
// token endpoint error bodies.
func dropBridgedField(key string) bool {
	return key == "body" || strings.HasPrefix(key, "request.header.") || strings.HasPrefix(key, "response.header.")
}

type nopFormatter struct{}

func (nopFormatter) Format(*upstreamLogrus.Entry) ([]byte, error) {
	return nil, nil
}
