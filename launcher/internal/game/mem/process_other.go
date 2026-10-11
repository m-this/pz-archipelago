//go:build !windows

package mem

import "errors"

// ErrNotRunning means no process of that name exists. Outside Windows there
// is never one to attach to.
var ErrNotRunning = errors.New("the game is not running")

// Open needs Windows: attaching to the game is a Windows process API.
func Open(string) (Process, error) { return nil, ErrNotRunning }
