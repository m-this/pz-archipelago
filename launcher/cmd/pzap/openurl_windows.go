//go:build windows

package main

import (
	"context"
	"os/exec"
)

// openURL hands a URL to its Windows handler: the browser, or Steam for a
// steam:// one.
func openURL(ctx context.Context, url string) error {
	return exec.CommandContext(context.WithoutCancel(ctx), "rundll32", "url.dll,FileProtocolHandler", url).Start()
}
