# pz-archipelago

A one-exe packaging of tinkieZo's Planet Zoo randomizer for Archipelago.
`DRAFT.md` has the plan; the README has the layout.

## Go owns the game data

`gamedata/` holds every species, item and location, with its Archipelago id.
Nothing else declares one.

- The apworld reads `apworld/planetzoo/data/*.json`, which `make export` writes
  in the shape upstream's decoders expect. `TestCommittedExportIsCurrent`
  fails when the committed copy is stale.
- The client reads the tables directly. No `data.json` sits between them.
- A location stores what its kind needs and derives the rest. A species
  location names its species and level; its name, its apworld string id and
  its water, fence and domestication flags come from the species.
- `*_generated.go` is written by `gamedata/cmd/importapworld` from an upstream
  apworld. A correction goes into the importer, not the generated file, or
  the next import undoes it.
- `Validate` holds the invariants: unique ids and names, one permit per
  species, every species with its acquisition, breeding, release (unless
  domesticated) and each welfare level, and each effect setting only the field
  its kind names.

## The apworld is upstream's

The Python under `apworld/planetzoo/` is tinkieZo's, renamed to lowercase
modules so it imports on a case-sensitive file system. Change it only to
fix a bug; the gameplay design is theirs.

## Build

Use the Makefile. `make check` is what CI runs.
