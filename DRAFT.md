# pz-archipelago: draft

One Windows exe that sets up and runs a Planet Zoo Archipelago game. The
player double-clicks `pzap.exe`, types the room address and the slot name, and
the exe does the rest: it finds the game, installs the mod, starts PopTracker,
starts the game on the Archipelago scenario, and runs the client.

This is the same shape as [tf2-archipelago](https://github.com/m-this/tf2-archipelago),
without a game server to manage. The Planet Zoo randomizer itself is the work
of tinkieZo (Mizore) and LTSmash. This repository repackages it; it does not
replace the design.

## What exists today

Three repositories, each installed by hand.

| Repository | What it is | Language | License |
| --- | --- | --- | --- |
| [tinkieZo/ArchipelagoPZ](https://github.com/tinkieZo/ArchipelagoPZ) | The apworld, in `worlds/planetzoo` of an Archipelago fork | Python | MIT |
| [tinkieZo/Planet-Zoo-AP](https://github.com/tinkieZo/Planet-Zoo-AP) | The mod and the client: ~10.7k lines | Python, Lua | GPL-3.0 |
| [tinkieZo/Planet-Zoo-Poptracker](https://github.com/tinkieZo/Planet-Zoo-Poptracker) | The PopTracker pack | Lua, JSON | MIT |

The player's steps today, from `setup_en.md`:

1. Download the client zip (PyInstaller, ~85 MB, a bundled Archipelago tree and
   a bundled cobra-tools).
2. Type `/pz_install`. The client builds the mod on the player's machine with
   cobra-tools: a content pack in `win64\ovldata\PZArchipelago\`, and one Lua
   module injected into `Content0\Main.ovl` (about 4 minutes). It backs up the
   vanilla file to `Main.ovl.apbak`.
3. Start the game, open Career, pick "ARCHIPELAGO ZOO", Start New.
4. Pause at once, go back to the client, connect, wait for READY. Touch nothing
   while it runs its preflight.
5. Play. Close the client before the game.
6. `/pz_restore` to go back to vanilla.

PopTracker is a separate download, and the pack a third one.

### How the client works

- `pz_ap_client/client.py` subclasses Archipelago's `CommonContext`, and so
  needs the whole Archipelago tree at run time.
- `pz_ap_client/memory/` attaches to the game with pymem. `scanner.py` does AOB
  scans and pointer chains, `anchors.json` holds the signatures, and `hook.py`
  detours code into trampolines for the events that have no stable address
  (births, releases, research, rewards). Each detector polls its scratch area
  and turns what it sees into location checks.
- `applier.py` and friends apply received items: permits, barrier levels,
  research centre, workshop, water tools, cash and conservation credits.
- `ovl.py` is the installer: a state machine (`no-game`, `vanilla`,
  `installed`, `stale`, `game-updated`, `ambiguous`) over the backup, a stamp
  file with hashes, and the `ovl_src/` sources. cobra-tools needs Oodle
  (`oo2core_8_win64.dll`); the client copies it from the game folder rather
  than ship it.
- `data.json` is the contract with the apworld: 78 species, 208 items, 745
  locations, item ids 10000-13005, location ids 20000-24443.

### What the Discord thread says

From the "Planet Zoo" thread in Future game design (Mizore, LTSmash, Ollieberry
and others, June to October 2026):

- The only goal is to breed a Giant Panda. Ollieberry asks for others: a zoo
  star rating, release X animals of a species, breed X random species, fully
  research X species. Mizore plans to make the goal configurable.
- No DLC animals. The DLC itself does not break a run.
- The continent, the climate and the terrain are fixed, because the scenario is
  built at install and is the same for every seed.
- Ollieberry sent a batch of checks just by connecting. LTSmash's reading: the
  client connected before the AP scenario was loaded. The manual order of
  steps 3 and 4 is where this comes from.
- A research centre placed before it is unlocked can be built but does not
  research. Releasing an animal before releases are unlocked plays the sound
  and does nothing.
- Logs go to `%LOCALAPPDATA%\PlanetZooAP\logs\`.

## What we build

```
pzap.exe
 ├─ finds Planet Zoo         Steam libraryfolders.vdf, app 703080
 ├─ installs the mod         pack + Content0 inject, backup, stamp, restore
 ├─ installs PopTracker      pinned release + the pack, opened on the right variant
 ├─ starts the game          steam://run/703080//-skipScenarioIntro/
 ├─ waits for the scenario   the "ARCHIPELAGO ZOO" park marker, read from memory
 ├─ runs the client          Archipelago websocket <-> memory hooks
 └─ serves the page          127.0.0.1, one embedded HTML page, opened in the browser
```

The order that the player handles by hand today becomes the program's job.
The client does not connect to the room until it sees the AP scenario loaded,
which removes the "checks sent on connect" report at its cause. It restores
its hooks when the game exits, so the close order stops mattering.

## No Python on the player's machine

Everything the player runs moves to Go. The apworld stays in Python because
Archipelago loads worlds as Python; it runs on the generator and the server,
never on the player's PC.

| Python today | Go | Notes |
| --- | --- | --- |
| `client.py` (`CommonContext`) | `internal/apclient` | Reuse tf2's `bridge/internal/apclient`: Connect, DataPackage, LocationChecks, ReceivedItems, Bounce for DeathLink |
| `data.py`, `data.json`, `tools/build_data_json.py` | `gamedata/` | Typed tables, no `data.json`; see below |
| `state.py` | `internal/state` | Item high-water mark, so an item is applied once |
| `memory/scanner.py` | `internal/memory` | `golang.org/x/sys/windows`: OpenProcess, Read/WriteProcessMemory, VirtualQueryEx |
| `memory/hook.py` | `internal/memory/hook` | Allocate near, suspend, write the detour, restore all; same shared 64 KB arena |
| `memory/anchors.py`, `anchors.json` | `internal/memory/anchors` | `anchors.json` embedded, unchanged format |
| `memory/{animals,births,releases,research,rewards,...}.py` | `internal/detect/*` | One file per detector, same names |
| `memory/applier.py`, `effects.py` | `internal/apply` | |
| `ovl.py` | `internal/install` | State machine, backup, stamp, restore |
| cobra-tools | `internal/ovl`, `internal/shell`, `internal/install` | See below |
| Kivy GUI | `internal/web` | One HTML page and a small JSON API; no frontend build |
| `tools/memscan.py` | `cmd/memscan` | Needed whenever a game patch moves a signature |

### Go owns the game data

As in tf2-archipelago, one Go package, `gamedata/`, declares every species,
item and location with its Archipelago id. The apworld reads JSON that
`make export` writes from it, in the exact shape upstream's decoders expect,
and the client reads the tables directly. The Python client's `data.json`
and the script that built it are gone: the effect each item has and the
trigger each location waits for are fields of the tables.

`gamedata/cmd/importapworld` reads an upstream `worlds/planetzoo` and rewrites
the generated tables, which is how upstream changes arrive. The first import
was checked both ways: the export equals upstream's JSON, and every effect
and trigger equals the `data.json` the Python client ships.

### The installer, without cobra-tools

The upstream installer does more than inject one Lua module. With
cobra-tools, it:

- builds the content pack and its `Loc.ovl`;
- in `Content0\Main.ovl`, replaces `scenarioscriptutils.lua` and rewrites six
  SQLite databases (`c0research`, `c0habitatboundary`, `c0modularscenery`,
  `c0blueprints`, `c0notifications`) to put a gate in front of every
  mechanic-research content;
- in `GameMain\Main.ovl`, renames `scenario_04.scenariomechanicresearchsettings`
  onto the AP scenario and adds the gate names to
  `default_off_noneresearchable.mechanicresearchsettings`.

`launcher/internal/ovl` does it in Go, without rebuilding the archive as cobra
does. It keeps each pool as opaque bytes plus its fragment table and changes
only what an edit needs:

- **Replacing a file's buffers** (the databases, the Lua module) changes no
  pool, so no offset moves.
- **Growing a pool** (the research topology) moves the later pools, the archive
  ranges and the stream entries by the growth. Pointers to the old end of the
  pool stay at the end.
- **Renaming** re-sorts that extension's files by hash and renumbers every
  index into the file table, in every archive.

Untouched archives keep their compressed bytes. An archive that does not
encode back to the bytes it was read from is refused before any edit.

cobra-tools is the oracle for the tests. Run on Linux, it wrote the fixtures:
our own pack, a Content0 and a GameMain in miniature, and its own inject and
rename of them. The Go results match those per file. The pack builder matches
cobra's `new` table by table.

Oodle is the game's `oo2core_8_win64.dll`, loaded through
`golang.org/x/sys/windows` with cobra's settings (Kraken, level 5, 8 MB
blocks, a seek reset every 256 KB). New files are zlib, as cobra writes them
for Planet Zoo. The databases go through `modernc.org/sqlite`, which needs no
C toolchain.

Not verified yet, because it needs the game:

- that the real `Content0` and `GameMain` read and re-encode exactly;
- whether they are Oodle-compressed;
- whether either has a `.ovs` the edits shift;
- that the game loads the result.

The first run on a machine with the game answers all four. `pzap status`
reads the files without changing them.

### The client, ported

`launcher/internal/game` is upstream's `memory/` package in Go: `mem` reads
and writes the game process (Windows API, and a fake for the tests), `hook`
builds the trampolines and places the detours, `pz` holds every address,
reader, detector and gate. `launcher/internal/play` is the loop `client.py`
ran: once a second, while the loaded park is the AP one, it detects checks,
applies items, settles the money ledger and reconciles every gate.

Two dead parts of upstream were left out: the placement `FacilityGate`, whose
hook site was never found and which no item reaches, and the `PresenceGate`,
which `client.py` creates but never reconciles.

The trampolines, the bytecode patches and the hashes match the Python output
byte for byte. The readers and gates are tested on a fake process laid out as
upstream describes the game, and the loop end to end on a fake game and a
fake room. What only the game can confirm is the same as for the installer:
that the addresses are still right for the current game build.

## PopTracker

The pack stays Lua: PopTracker runs Lua packs and nothing else. The exe
downloads a pinned PopTracker release, checks it, puts the pack in its `packs`
folder, and starts it on the "Map Tracker" variant with `--ap-host`,
`--ap-slot` and `--ap-password`, so the pack connects to the room without the
player typing it twice.

The pack's `item_mapping.lua` and `location_mapping.lua` repeat the ids; they
should be generated from `gamedata/` too, with the launcher work.

A web tracker on GitHub Pages, like tf2's, is a later option. It does not
replace PopTracker.

## Repository layout

Same split as tf2-archipelago.

```
gamedata/        the tables, the export, the upstream importer
apworld/         the world, imported from ArchipelagoPZ worlds/planetzoo
tracker/         the PopTracker pack, imported from Planet-Zoo-Poptracker
mod/             the Lua and loc sources the installer builds (upstream's ovl_src)
launcher/        pzap.exe: cmd/pzap, internal/*, the page in internal/web
docs/            the site: en/ and fr/, published to GitHub Pages
Makefile         make test lint launcher apworld docs
```

One Go module. `make launcher` cross-compiles `pzap.exe` from Linux.

## The site

GitHub Pages, as for tf2: a landing page with the download button, the setup
guide in English and French, the options, the goal, and a "what the
randomizer changes" page. The release publishes `pzap.exe`, the `.apworld`,
`SHA256SUMS` and a VirusTotal link. An exe that writes into another process's
memory will be flagged by scanners; the README says so up front, as tf2's does.

## Milestones

1. **Skeleton.** Done. Go module, Makefile, CI, `gamedata/` with its export
   and importer, the apworld, the pack and the mod sources imported, the
   apworld tested inside Archipelago 0.6.8, a seed generated from the packaged
   `.apworld`.
2. **Installer.** Written: Steam detection, Oodle, OVL read and write, the
   shell edits, the install state machine, restore, `pzap status|install|restore`.
   Done when a Go-built install loads the "ARCHIPELAGO ZOO" scenario on a real
   Windows machine.
3. **Client.** Written: the Archipelago connection, the memory layer, every
   detector, gate and effect the Python client runs, `pzap play`. Done when one
   seed plays to the goal with the same checks the Python client sends, on the
   same save.
4. **Launcher.** Written: double-clicking `pzap.exe` serves the page on
   127.0.0.1 and opens it; the page installs and restores the mod, saves the
   room settings, connects, launches the game through Steam, downloads and
   opens PopTracker, and shows the room's messages and the log, in English
   or French. Left: a test mode with tf2's fake room, and the debug bundle.
5. **Release.** Written: the site in `docs/`, deployed to GitHub Pages from
   main, and a release workflow on semver tags that publishes `pzap.exe`, the
   `.apworld`, the PopTracker pack, `SHA256SUMS` and a build attestation.
6. **Goals.** The goals the thread asks for, in the apworld.

## Licensing and credit

- The client is GPL-3.0. A Go port of it is a derived work, so this repository
  is GPL-3.0.
- The apworld and the pack are MIT; their notices stay with them.
- The README names tinkieZo and LTSmash as the authors of the randomizer and
  links their repositories.

## Open questions

- Linux through Proton: Planet Zoo runs under it; the memory layer would need
  the game's process from inside the Wine prefix. Out of scope for the first
  release.
- Upstream: the apworld's data comes in through the importer, and its Python
  by hand. The pack has no such path yet.
