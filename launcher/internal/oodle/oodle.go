// Package oodle compresses and decompresses with the Oodle library Planet
// Zoo ships, oo2core_8_win64.dll. The DLL is RAD's and is loaded from the
// player's own game folder, never redistributed. The settings mirror
// cobra-tools' modules/formats/utils/oodle/oodle.py, which the game reads.
package oodle

// DLLName is the file in the Planet Zoo folder.
const DLLName = "oo2core_8_win64.dll"
