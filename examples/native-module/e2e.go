//go:build e2e

package main

// Builds with -tags e2e can be driven through the automation plugin:
// MYGO_AUTOMATION=<socket> ./app, then mygo-automation.
import _ "github.com/egoist/mygo/plugins/automation"
