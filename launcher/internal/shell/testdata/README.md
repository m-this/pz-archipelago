# Test fixtures

`content0.ovl` is a Content0 in miniature: cobra-tools (`fe5d39d`, `new`,
Planet Zoo) over the five databases the shell edits, each with only the
columns it touches and a few rows, and a one-line
`scenarioscripts.scenarioscriptutils.lua`. `shell_test.go` lists the rows it
expects after the patch.
