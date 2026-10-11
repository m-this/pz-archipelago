# pz-archipelago

A one-exe packaging of the [Planet Zoo](https://store.steampowered.com/app/703080/Planet_Zoo/)
randomizer for [Archipelago](https://archipelago.gg). The randomizer is the work
of tinkieZo and LTSmash: the
[apworld](https://github.com/tinkieZo/ArchipelagoPZ), the
[mod and client](https://github.com/tinkieZo/Planet-Zoo-AP) and the
[PopTracker pack](https://github.com/tinkieZo/Planet-Zoo-Poptracker). This
repository repackages it so a player runs one file.

Not playable yet. [DRAFT.md](./DRAFT.md) has the plan and the milestones.

## Layout

| Directory | Holds |
| --- | --- |
| `gamedata/` | Go. Every species, item and location, and every Archipelago id. The source of truth |
| `apworld/planetzoo/` | The Archipelago world. Its `data/` is exported from `gamedata/` |
| `tracker/` | The PopTracker pack |
| `mod/` | The Lua and loc sources the installer builds the game files from |
| `launcher/` | `pzap.exe` and the tools that build it |

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
