# YuE2 score fixtures

`planned.abc` is a song score in YuE2's two-voice ABC dialect, written for cav's tests (it
is not from YuE2). It has an intro, a verse, a chorus, a bridge in another key and an outro,
with Ins notes that overlap the Vocal notes.

The other files are the output of the Python code that cav's Go port in `yue2abc.go`
copies:

- `converted.abc`: `convert_score` of the instrumental helper in the YuE2 repository
  (`skills/yue2-music/instrumental/scripts`, commit 1647252).
- `fit-<seconds>.abc`: `fit_score` of cav's wrapper (`tools/yue2-server/yue2_server.py`).
- `expected.json`: the nominal lengths, the lyric tags and the instrumental style.

To make them again, from the cav repository, with a YuE2 checkout at `<YuE>`:

    python3 -I internal/music/testdata/yue2/make.py <YuE> internal/music/testdata/yue2 tools/yue2-server
