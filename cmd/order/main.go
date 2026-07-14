// Command order is the entry point for the Order service. It wires
// version/commit (injected via ldflags) and delegates all startup logic
// to the app package (internal/order/app), keeping only os.Exit here.
package main

import (
	"fmt"
	"os"

	"github.com/devguy201-9/checkout-saga/internal/order/app"
)

// version/commit are set via ldflags at build time:
//
//	-X main.version=$(VERSION) -X main.commit=$(COMMIT)
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := app.Run(version, commit); err != nil {
		// Exit only in main (never os.Exit from deep in the code). The error is
		// already structured-logged inside app.Run once the logger is up; stderr
		// is the fallback for failures before the logger is ready (e.g. config load).
		fmt.Fprintln(os.Stderr, "order-service fatal:", err)
		os.Exit(1)
	}
}
