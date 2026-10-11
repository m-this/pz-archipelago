//go:build !windows

package main

import (
	"context"
	"os/exec"
)

func openURL(ctx context.Context, url string) error {
	return exec.CommandContext(context.WithoutCancel(ctx), "xdg-open", url).Start()
}
