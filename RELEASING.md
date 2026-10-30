# Releasing

nowplaying is installed with Homebrew from a tap: a repository named
`homebrew-tap` that holds the formula.

## One-time setup

1. Push this repository to `github.com/kraklin/nowplaying-tui`.
2. Create a public repository `github.com/kraklin/homebrew-tap` with a `Formula/`
   folder.

## Each release

1. Make sure `go test -race ./...` passes, then tag and push:

   ```sh
   jj bookmark set main -r @-        # or wherever the release commit is
   jj git push
   git tag v0.1.0 <commit>            # jj has no tags yet; use git for this
   git push origin v0.1.0
   ```

2. Get the checksum of the tag's tarball:

   ```sh
   curl -sL https://github.com/kraklin/nowplaying-tui/archive/refs/tags/v0.1.0.tar.gz | shasum -a 256
   ```

3. Copy `packaging/homebrew/nowplaying.rb` to `Formula/nowplaying.rb` in the tap, set
   the `url` (tag) and `sha256`, then check it:

   ```sh
   brew install --build-from-source kraklin/tap/nowplaying
   brew test kraklin/tap/nowplaying
   brew audit --strict --online kraklin/tap/nowplaying
   ```

4. Commit and push the tap. Users then run `brew install kraklin/tap/nowplaying`.

The version printed by `nowplaying --version` comes from the formula
(`-X main.version=#{version}`); a build without it prints `dev`, and
`go install github.com/kraklin/nowplaying-tui/cmd/nowplaying@v0.1.0` prints the
module version.
