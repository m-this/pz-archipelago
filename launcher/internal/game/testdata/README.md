# Test fixtures

`python-golden.json` is what tinkieZo's Planet-Zoo-AP (`896e030`) returns, run
on Linux with `ctypes.WinDLL` stubbed, for:

- every trampoline builder the client uses (`memory/hook.py`), for region and
  scratch `0x150010000`, resume `0x140C8216F` and fail `0x140C82100`, with each
  site's original bytes;
- the exhibit enrichment bytecode variants 0 to 4 (`memory/exhibit_enrich.py`)
  and `LOADBOOL` for registers 4, 5 and 200 (`memory/terrain.py`);
- the market include-set hash and set images for random keys
  (`memory/market.py`), and the animal roster hash (`memory/animals.py`).

The Go tests in `hook` and `pz` must produce the same bytes.
