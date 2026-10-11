# pz-archipelago

A one-exe packaging of the [Planet Zoo](https://store.steampowered.com/app/703080/Planet_Zoo/)
randomizer for [Archipelago](https://archipelago.gg). The randomizer is the work
of tinkieZo and LTSmash: the
[apworld](https://github.com/tinkieZo/ArchipelagoPZ), the
[mod and client](https://github.com/tinkieZo/Planet-Zoo-AP) and the
[PopTracker pack](https://github.com/tinkieZo/Planet-Zoo-Poptracker). This
repository repackages it so a player runs one file.

Not playable yet: the installer and the client are written and untested on a
real game. [DRAFT.md](./DRAFT.md) has the plan and the milestones.

Double-click `pzap.exe`. It opens a page in the browser that installs the
mod, connects to the room, starts the game and opens PopTracker. Keep its
window open while playing; closing it removes the client's patches from the
game.

The same steps run from a terminal:

```sh
pzap status     # is the AP scenario shell installed?
pzap install    # install it, or bring it up to date (close the game first)
pzap restore    # put the vanilla game files back
pzap play -room archipelago.gg:38281 -slot NAME
```

`-game-dir` points at the Planet Zoo folder when Steam does not know it.

## Layout

| Directory | Holds |
| --- | --- |
| `gamedata/` | Go. Every species, item and location, and every Archipelago id. The source of truth |
| `apworld/planetzoo/` | The Archipelago world. Its `data/` is exported from `gamedata/` |
| `tracker/` | The PopTracker pack |
| `mod/` | The Lua and loc sources the installer builds the game files from |
| `launcher/internal/ovl` | Reads and writes Planet Zoo's OVL archives |
| `launcher/internal/shell` | What the installer changes in the game files |
| `launcher/internal/install` | Backups, the install receipt, install and restore |
| `launcher/internal/game` | Reading and patching the running game |
| `launcher/internal/play` | The client loop: items in, checks out |
| `launcher/internal/web` | The page the player sees |
| `launcher/` | `pzap.exe` and the rest of what it needs |

## Build

Use the Makefile; `make help` lists every target.

```sh
make check          # everything CI runs
make dist           # pzap.exe, planetzoo.apworld, the PopTracker pack
make apworld-test   # the apworld's tests inside Archipelago, through uv
```

The game data starts in `gamedata/`. After a change to a table, `make export`
rewrites the apworld's copy, and a test fails if that step was skipped. When
the upstream apworld changes, `make import UPSTREAM=<ArchipelagoPZ>/worlds/planetzoo`
rewrites the tables from it; read the diff before keeping it.

## License

GPL-3.0, because the client this project ports is GPL-3.0. The apworld
(`apworld/planetzoo/LICENSE`) and the PopTracker pack (`tracker/LICENSE`) keep
their MIT licenses.
