# Test fixtures

`EmptyPZ.ovl` and `OVLIncludesPZ.ovl` come from cobra-tools' own tests
(`tests/ovldata`). Every `cobra-*.ovl` was written by cobra-tools at commit
`fe5d39d`, on Linux, without Oodle, so Planet Zoo files come out
zlib-compressed:

```sh
python ovl_tool_cmd.py new -g "Planet Zoo" -i <dir> -o <out.ovl> --force
python ovl_tool_cmd.py inject -g "Planet Zoo" -i <dir> -o <out.ovl> <in.ovl>
```

| Fixture | Made from |
| --- | --- |
| `cobra-pack.ovl` | `new` over `mod/pack` |
| `cobra-loc.ovl` | `new` over `mod/pack_loc` |
| `cobra-c0-vanilla.ovl` | `new` over a 20-row `c0research.fdb`, a two-line `scenarioscripts.scenarioscriptutils.lua` and a `.txt` |
| `cobra-c0-injected.ovl` | `inject` of `c0research-new.fdb` and `scriptutils-new.lua` into `cobra-c0-vanilla.ovl` |
| `cobra-gm-vanilla.ovl` | `new` over five `.mechanicresearchsettings` (XML), the fdb and the lua above |
| `cobra-gm-patched.ovl` | cobra's `OvlFile.rename` of `scenario_04` onto `scenario_22_empty`, then `add_files` of the topology with three gates more, then `save` |

cobra-tools lays roots out in the iteration order of a Python set, so its
pools differ from run to run. The tests compare per file, never whole pools.
