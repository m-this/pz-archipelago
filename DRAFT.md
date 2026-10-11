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
 └─ serves the page          127.0.0.1, embedded Angular app, opened in the browser
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
| `data.py`, `data.json` | `internal/gamedata` | `data.json` embedded, validated at start |
| `state.py` | `internal/state` | Item high-water mark, so an item is applied once |
| `memory/scanner.py` | `internal/memory` | `golang.org/x/sys/windows`: OpenProcess, Read/WriteProcessMemory, VirtualQueryEx |
| `memory/hook.py` | `internal/memory/hook` | Allocate near, suspend, write the detour, restore all; same shared 64 KB arena |
| `memory/anchors.py`, `anchors.json` | `internal/memory/anchors` | `anchors.json` embedded, unchanged format |
| `memory/{animals,births,releases,research,rewards,...}.py` | `internal/detect/*` | One file per detector, same names |
| `memory/applier.py`, `effects.py` | `internal/apply` | |
| `ovl.py` | `internal/install` | State machine, backup, stamp, restore |
| cobra-tools | `internal/ovl` | See below |
| Kivy GUI | `web/` | Angular, same stack as the tf2 launcher |
| `tools/memscan.py` | `cmd/memscan` | Needed whenever a game patch moves a signature |

### cobra-tools is the one hard part

cobra-tools is a large Python project that reads and writes Frontier's OVL
archives. We need a small part of it:

1. `new`: build a small OVL from a handful of Lua files and loc strings (the
   content pack and its `Loc.ovl`).
2. `inject`: replace one Lua module in `Content0\Main.ovl`, and rename one
   mechanic-research settings entry in `GameMain`.

The plan is a Go reader and writer for that subset, for the OVL version Planet
Zoo ships, with Oodle loaded from the game's own `oo2core_8_win64.dll` through
`windows.LoadDLL`. Nothing from Frontier or RAD goes into the repository or the
release. Test fixtures are OVLs we build from our own sources; the round trip
against the real `Main.ovl` runs on a machine with the game.

If the Go writer cannot be made byte-correct in reasonable time, the fallback
is the only case where Python stays: the exe downloads the official CPython
embeddable zip and a pinned cobra-tools commit on first install, checks their
SHA-256, and runs cobra as a hidden subprocess. The player still sees one exe
and one button. This is a fallback, not the plan.

A side effect of the Go writer: the inject may stop taking 4 minutes. cobra
repacks the whole archive; a writer that replaces one module may not need to.
To measure, not to promise.

## PopTracker

The pack stays Lua: PopTracker runs Lua packs and nothing else. The exe
downloads a pinned PopTracker release, checks it, puts the pack in its `packs`
folder, and starts it on the "Map Tracker" variant. The pack connects to the
room itself. Whether PopTracker can be told the room address and slot on its
command line needs checking; if not, the page shows them with a copy button.

A web tracker on GitHub Pages, like tf2's, is a later option. It does not
replace PopTracker.

## Repository layout

Same split as tf2-archipelago.

```
apworld/         the world, imported from ArchipelagoPZ worlds/planetzoo
tracker/         the PopTracker pack, imported from Planet-Zoo-Poptracker
mod/             ovl_src: the Lua and loc sources the installer builds
launcher/        pzap.exe: cmd/pzap, internal/*, web/ (Angular)
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

1. **Skeleton.** Go module, Makefile, CI, the apworld and the pack imported
   with their history credited, the Python apworld tests running in CI.
2. **Installer.** Steam detection, Oodle loading, OVL read and write, the
   install state machine, restore. Done when a Go-built install loads the
   "ARCHIPELAGO ZOO" scenario on a real Windows machine.
3. **Client.** Archipelago connection, the memory layer, every detector and
   every effect. Done when one seed plays to the goal with the same checks the
   Python client sends, on the same save.
4. **Launcher.** The page, PopTracker, the game launch, the wait for the
   scenario, a test mode with tf2's fake room, the debug bundle.
5. **Release.** The site, the release workflow, nightly builds.
6. **Goals.** The goals the thread asks for, in the apworld.

## Licensing and credit

- The client is GPL-3.0. A Go port of it is a derived work, so this repository
  is GPL-3.0.
- The apworld and the pack are MIT; their notices stay with them.
- The README names tinkieZo and LTSmash as the authors of the randomizer, and
  links their repositories and the Discord thread.

## Open questions

- Does PopTracker take a room address and slot on its command line?
- Which OVL version and Oodle level does the current Planet Zoo build use, and
  how much of cobra's writer is needed for `inject` to load?
- Linux through Proton: Planet Zoo runs under it; the memory layer would need the game's process from inside
  the Wine prefix. Out of scope for the first release.
- Track upstream how? The apworld and the pack change under us; a sync script
  that diffs `worlds/planetzoo` and the pack against their `main` is probably
  enough.
