//go:build !windows

package install

// refuseWhileRunning has no process list to read outside Windows; the game
// runs there under Proton, which a later version may look into.
func (Game) refuseWhileRunning() error { return nil }
