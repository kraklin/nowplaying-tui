# testdata

Raw `media-control` 0.7.7 output (macOS 26, Spotify desktop), the source of truth for the parser.

- `stream.jsonl` — `media-control stream --micros`, ~12 s: initial state, pause, resume, next track, seek to 20 s.
- `get.json` — `media-control get --micros --no-artwork`.

What the recordings show (the brief had guessed otherwise):

- Each stream line is an envelope: `{"type":"data","diff":bool,"payload":{...}}`.
- `diff:false` replaces the whole state; `diff:true` merges keys, and a `null` value clears that key.
- The first line is `payload: {}` when nothing is playing.
- Without `--micros`, `timestamp` is ISO-8601 with 1 s resolution; `--micros` gives `timestampEpochMicros`, `elapsedTimeMicros`, `durationMicros`.
- Artwork often arrives in its own diff after the track change, so a track can briefly have no art.
- On pause `playing` goes false and `playbackRate` becomes null; on resume `playing` goes true a couple of lines *before* `playbackRate` is back.
- `contentItemIdentifier` and `timestamp` change on many lines with no visible effect.
- Not recorded: the `null` that `get` prints when nothing is playing.
