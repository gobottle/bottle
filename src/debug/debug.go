package debug

import (
	"log"
	"time"
)

var ShouldTimeFunctions bool

func TimedFunction(start time.Time, funcName string) {
	if ShouldTimeFunctions {
		elapsed := time.Since(start)
		log.Printf("%15s: %s", elapsed, funcName)
	}
}
