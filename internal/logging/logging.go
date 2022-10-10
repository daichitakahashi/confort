package logging

import (
	"fmt"
	"log"
	"path"
	"runtime"
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
)

func file(depth int) string {
	pc, filename, line, _ := runtime.Caller(depth)
	funcName := path.Base(runtime.FuncForPC(pc).Name())
	return fmt.Sprintf("\n\t%s:%d:%s", filename, line, funcName)
}

func Debug(args ...any) {
	if LevelDebug.enabled(level) {
		log.Println(debugPrefix, fmt.Sprint(args...), file(2))
	}
}

func Debugf(format string, args ...any) {
	if LevelDebug.enabled(level) {
		log.Println(debugPrefix, fmt.Sprintf(format, args...), file(2))
	}
}

func Info(args ...any) {
	if LevelInfo.enabled(level) {
		log.Println(infoPrefix, fmt.Sprint(args...))
	}
}

func Infof(format string, args ...any) {
	if LevelInfo.enabled(level) {
		log.Println(infoPrefix, fmt.Sprintf(format, args...))
	}
}
