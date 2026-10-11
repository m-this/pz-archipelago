//go:build windows

package install

import (
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const exeName = "PlanetZoo.exe"

// refuseWhileRunning stops an install or a restore while the game has its
// files open.
func (g Game) refuseWhileRunning() error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), exeName) {
			return errors.New("planet zoo is running; close the game first")
		}
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil
	}
	return err
}
