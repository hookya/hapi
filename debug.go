package hapi

import (
	"fmt"
	"os"
	"strings"
)

// EnvHApiMode indicates environment name for HApi mode.
const EnvMode = "MODE"

const (
	debugCode = iota
	releaseCode
	testCode
)

var (
	Mode     = debugCode
	modeName = DebugMode
)

const (
	// DebugMode indicates HApi mode is debug.
	DebugMode = "debug"
	// ReleaseMode indicates HApi mode is release.
	ReleaseMode = "release"
	// TestMode indicates HApi mode is test.
	TestMode = "test"
)

func init() {
	mode := os.Getenv(EnvMode)
	SetMode(mode)
}

func IsDebugging() bool {
	return Mode == debugCode
}

func debugPrintRoute(httpMethod, absolutePath string, handlers HandlersChain) {
	if IsDebugging() {
		nuHandlers := len(handlers)
		handlerName := nameOfFunction(handlers.Last())
		if DebugPrintRouteFunc == nil {
			debugPrint("%-6s %-25s --> %s (%d handlers)\n", httpMethod, absolutePath, handlerName, nuHandlers)
		} else {
			DebugPrintRouteFunc(httpMethod, absolutePath, handlerName, nuHandlers)
		}
	}
}

func SetMode(value string) {
	if value == "" {
		value = DebugMode
	}

	switch value {
	case DebugMode:
		Mode = debugCode
	case ReleaseMode:
		Mode = releaseCode
	case TestMode:
		Mode = testCode
	default:
		panic("hapi mode unknown: " + value + " (available mode: debug release test)")
	}

	modeName = value
}

// DebugPrintRouteFunc indicates debug log output format.
var DebugPrintRouteFunc func(httpMethod, absolutePath, handlerName string, nuHandlers int)

func debugPrint(format string, values ...any) {
	if IsDebugging() {
		if !strings.HasSuffix(format, "\n") {
			format += "\n"
		}
		fmt.Fprintf(os.Stdout, "[hapi-debug] "+format, values...)
	}
}
