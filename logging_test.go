package clio

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/fangs"
	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/discard"
	"github.com/anchore/go-logger/adapter/redact"
)

func Test_newLogger(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *LoggingConfig
		store        redact.Store
		assertLogger func(logger.Logger)
		wantErr      require.ErrorAssertionFunc
	}{
		{
			name: "no log config still has a logger",
			cfg:  nil,
			assertLogger: func(log logger.Logger) {
				require.NotNil(t, log)
				c, ok := log.(logger.Controller)
				if !ok {
					t.Fatal("expected logger to be a controller")
				}

				buf := &bytes.Buffer{}
				c.SetOutput(buf)
				log.Info("test")

				// prove this is a nil logger
				assert.Equal(t, "", buf.String())
			},
		},
		{
			name: "log config creates a logger",
			cfg:  &LoggingConfig{Level: "debug"},
			assertLogger: func(log logger.Logger) {
				require.NotNil(t, log)
				c, ok := log.(logger.Controller)
				if !ok {
					t.Fatal("expected logger to be a controller")
				}

				buf := &bytes.Buffer{}
				c.SetOutput(buf)
				log.Info("test")

				// prove this is a NOT a nil logger
				assert.Equal(t, "[0000]  INFO test\n", stripAnsi(buf.String()))
			},
		},
		{
			name:  "adds redactor",
			cfg:   &LoggingConfig{Level: "debug"},
			store: redact.NewStore("secret"),
			assertLogger: func(log logger.Logger) {
				require.NotNil(t, log)
				c, ok := log.(logger.Controller)
				if !ok {
					t.Fatal("expected logger to be a controller")
				}

				buf := &bytes.Buffer{}
				c.SetOutput(buf)
				log.Info("test secret")

				// prove this is a NOT a nil logger
				assert.Equal(t, "[0000]  INFO test *******\n", stripAnsi(buf.String()))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			log, err := DefaultLogger(Config{Log: tt.cfg}, tt.store)
			tt.wantErr(t, err)
			if err != nil {
				return
			}
			tt.assertLogger(log)
		})
	}
}

func TestLoggingConfig_AddFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
	}{
		{
			name: "flags are registered",
			flags: map[string]string{
				"quiet":   "q",
				"verbose": "v",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &LoggingConfig{}
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			l.AddFlags(fangs.NewPFlagSet(discard.New(), flags))

			var actual = make(map[string]string)
			flags.VisitAll(func(flag *pflag.Flag) {
				actual[flag.Name] = flag.Shorthand
				assert.NotEmpty(t, flag.Usage)
			})

			assert.Equal(t, tt.flags, actual)
		})
	}
}

var _ fs.File = (*fakeFile)(nil)
var _ fs.FileInfo = (*fakeInfo)(nil)

type fakeInfo struct {
	mode fs.FileMode
	size int64
}

func (f fakeInfo) Mode() fs.FileMode { return f.mode }

func (f fakeInfo) Size() int64 { return f.size }

func (f fakeInfo) Name() string { panic("not implemented") }

func (f fakeInfo) ModTime() time.Time { panic("not implemented") }

func (f fakeInfo) IsDir() bool { panic("not implemented") }

func (f fakeInfo) Sys() any { panic("not implemented") }

type fakeFile struct {
	info fs.FileInfo
	err  error
}

func (f *fakeFile) Stat() (fs.FileInfo, error) {
	return f.info, f.err
}

func (f *fakeFile) Read([]byte) (int, error) { panic("not implemented") }

func (f *fakeFile) Close() error { panic("not implemented") }

var _ terminalDetector = (*mockTerminalDetector)(nil)

type mockTerminalDetector struct {
	stdout bool
	stderr bool
}

func (m mockTerminalDetector) StdoutIsTerminal() bool {
	return m.stdout
}

func (m mockTerminalDetector) StderrIsTerminal() bool {
	return m.stderr
}

func TestLoggingConfig_AllowUI(t *testing.T) {

	tests := []struct {
		name  string
		cfg   *LoggingConfig
		stdin fs.File
		want  bool
	}{
		{
			name:  "no config, no stdin = allowed",
			cfg:   nil,
			stdin: nil,
			want:  true,
		},
		{
			name:  "no config, stdin piped input = not allowed",
			cfg:   nil,
			stdin: &fakeFile{info: fakeInfo{mode: fs.ModeNamedPipe}},
			want:  false,
		},
		{
			name: "non-verbose config, stdin piped input = not allowed",
			cfg: &LoggingConfig{
				Verbosity: 0,
				terminalDetector: mockTerminalDetector{
					stdout: true,
					stderr: true,
				},
			},
			stdin: &fakeFile{info: fakeInfo{mode: fs.ModeNamedPipe}},
			want:  false,
		},
		{
			name: "verbose config, stdin w/o piped input = not allowed",
			cfg: &LoggingConfig{
				Verbosity: 1,
				terminalDetector: mockTerminalDetector{
					stdout: true,
					stderr: true,
				},
			},
			stdin: &fakeFile{info: fakeInfo{mode: 0}},
			want:  false,
		},
		{
			name: "non-verbose config, stdin w/o piped input and has bytes waiting = allowed",
			cfg: &LoggingConfig{
				Verbosity: 0,
				terminalDetector: mockTerminalDetector{
					stdout: true,
					stderr: true,
				},
			},
			stdin: &fakeFile{
				info: fakeInfo{
					mode: 0,
					size: 100,
				},
			},
			want: true,
		},
		{
			name: "non-verbose config, stdin w/o piped input and no bytes waiting = allowed",
			cfg: &LoggingConfig{
				Verbosity: 0,
				terminalDetector: mockTerminalDetector{
					stdout: true,
					stderr: true,
				},
			},
			stdin: &fakeFile{info: fakeInfo{mode: 0}},
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.AllowUI(tt.stdin))
		})
	}
}

func Test_isPipedInput(t *testing.T) {

	tests := []struct {
		name    string
		stdin   fs.File
		want    bool
		wantErr require.ErrorAssertionFunc
	}{
		{
			name:  "nil stdin",
			stdin: nil,
			want:  false,
		},
		{
			name:    "stat error",
			stdin:   &fakeFile{err: errors.New("stat error")},
			wantErr: require.Error,
		},
		{
			name:  "not a pipe",
			stdin: &fakeFile{info: fakeInfo{mode: 0}},
			want:  false,
		},
		{
			name:  "is a pipe",
			stdin: &fakeFile{info: fakeInfo{mode: fs.ModeNamedPipe}},
			want:  true,
		},
		{
			name:  "is not a pipe, but stdin size > 0",
			stdin: &fakeFile{info: fakeInfo{mode: 0, size: 100}},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			got, err := isPipedInput(tt.stdin)
			tt.wantErr(t, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoggingConfig_selectLevel(t *testing.T) {

	tests := []struct {
		name    string
		cfg     *LoggingConfig
		want    logger.Level
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "no config",
			cfg:  nil,
			want: logger.WarnLevel,
		},
		{
			name: "empty config",
			cfg:  &LoggingConfig{},
			want: logger.InfoLevel,
		},
		{
			name: "quiet config",
			cfg:  &LoggingConfig{Quiet: true},
			want: logger.DisabledLevel,
		},
		{
			name: "verbose 1 config",
			cfg:  &LoggingConfig{Verbosity: 1},
			want: logger.InfoLevel,
		},
		{
			name: "verbose 2 config",
			cfg:  &LoggingConfig{Verbosity: 2},
			want: logger.DebugLevel,
		},
		{
			name: "verbose 3 config",
			cfg:  &LoggingConfig{Verbosity: 3},
			want: logger.TraceLevel,
		},
		{
			name: "verbose overflow config",
			cfg:  &LoggingConfig{Verbosity: 4},
			want: logger.TraceLevel,
		},
		{
			name: "verbose trumps level parsing",
			cfg:  &LoggingConfig{Verbosity: 4, Level: logger.WarnLevel},
			want: logger.TraceLevel,
		},
		{
			name: "verbose underflow config",
			cfg:  &LoggingConfig{Verbosity: -1},
			want: logger.InfoLevel,
		},
		{
			name: "verbose underflow trumped by level setting",
			cfg:  &LoggingConfig{Verbosity: -1, Level: logger.ErrorLevel},
			want: logger.ErrorLevel,
		},
		{
			name: "quiet trumps other options",
			cfg:  &LoggingConfig{Verbosity: 1, Quiet: true, Level: logger.WarnLevel},
			want: logger.DisabledLevel,
		},
		{
			name: "set level directly",
			cfg:  &LoggingConfig{Level: logger.ErrorLevel},
			want: logger.ErrorLevel,
		},
		{
			name:    "bogus level set",
			cfg:     &LoggingConfig{Level: logger.Level("bogosity")},
			want:    logger.DisabledLevel,
			wantErr: require.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			got, err := tt.cfg.selectLevel()
			assert.Equal(t, tt.want, got)
			tt.wantErr(t, err)
		})
	}
}
