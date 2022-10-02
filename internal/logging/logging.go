package logging

import (
	"fmt"
	"path"
	"runtime"
	"testing"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelError
)

var level = LevelError

func (l LogLevel) enabled(base LogLevel) bool {
	return base <= l
}

// SetLogLevel sets log level.
// This func is unsafe for concurrent use.
func SetLogLevel(l LogLevel) {
	level = l
}

var (
	debugPrefix = "confort[DEBUG]:"
	infoPrefix  = "confort[INFO]:"
	errorPrefix = "confort[ERROR]:"
	fatalPrefix = "confort[FATAL]:"
)

func file(depth int) string {
	pc, filename, line, _ := runtime.Caller(depth)
	funcName := path.Base(runtime.FuncForPC(pc).Name())
	return fmt.Sprintf("\n\t%s:%d:%s", filename, line, funcName)
}

func Debug(tb testing.TB, args ...any) {
	tb.Helper()
	if LevelDebug.enabled(level) {
		tb.Log(debugPrefix, fmt.Sprint(args...), file(2))
	}
}

func Debugf(tb testing.TB, format string, args ...any) {
	tb.Helper()
	if LevelDebug.enabled(level) {
		tb.Log(debugPrefix, fmt.Sprintf(format, args...), file(2))
	}
}

func Info(tb testing.TB, args ...any) {
	tb.Helper()
	if LevelInfo.enabled(level) {
		tb.Log(infoPrefix, fmt.Sprint(args...))
	}
}

func Infof(tb testing.TB, format string, args ...any) {
	tb.Helper()
	if LevelInfo.enabled(level) {
		tb.Log(infoPrefix, fmt.Sprintf(format, args...))
	}
}

func Error(tb testing.TB, args ...any) {
	tb.Helper()
	if LevelError.enabled(level) {
		tb.Log(errorPrefix, fmt.Sprint(args...), file(2))
	}
}

// func Errorf(tb testing.TB, format string, args ...any) {
// 	tb.Helper()
// 	if LevelError.enabled(level) {
// 		tb.Log(errorPrefix, fmt.Sprintf(format, args...), file(2))
// 	}
// }

func Fatal(tb testing.TB, args ...any) {
	tb.Helper()
	tb.Fatal(fatalPrefix, fmt.Sprint(args...), file(2))
}

func Fatalf(tb testing.TB, format string, args ...any) {
	tb.Helper()
	tb.Fatalf(fatalPrefix, fmt.Sprintf(format, args...), file(2))
}
