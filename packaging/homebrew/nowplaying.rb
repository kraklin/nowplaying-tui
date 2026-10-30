class Nowplaying < Formula
  desc "Pixel-art now-playing card for the terminal (macOS)"
  homepage "https://github.com/kraklin/nowplaying-tui"
  url "https://github.com/kraklin/nowplaying-tui/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000" # replace
  license "MIT"
  head "https://github.com/kraklin/nowplaying-tui.git", branch: "main"

  depends_on "go" => :build
  depends_on :macos
  depends_on "media-control"

  def install
    system "go", "build", *std_go_args(ldflags: "-X main.version=#{version}"), "./cmd/nowplaying"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/nowplaying --version")
  end
end
