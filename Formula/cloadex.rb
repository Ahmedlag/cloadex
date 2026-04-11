class Cloadex < Formula
  desc "CLI that orchestrates Claude Code and OpenAI Codex into a collaborative coding workflow"
  homepage "https://github.com/Ahmedlag/cloadex"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/Ahmedlag/cloadex/releases/download/v#{version}/cloadex-darwin-arm64.tar.gz"
      sha256 "PLACEHOLDER" # Run `make formula` to fill after release build
    end
    on_intel do
      url "https://github.com/Ahmedlag/cloadex/releases/download/v#{version}/cloadex-darwin-amd64.tar.gz"
      sha256 "PLACEHOLDER" # Run `make formula` to fill after release build
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/Ahmedlag/cloadex/releases/download/v#{version}/cloadex-linux-arm64.tar.gz"
      sha256 "PLACEHOLDER" # Run `make formula` to fill after release build
    end
    on_intel do
      url "https://github.com/Ahmedlag/cloadex/releases/download/v#{version}/cloadex-linux-amd64.tar.gz"
      sha256 "PLACEHOLDER" # Run `make formula` to fill after release build
    end
  end

  head "https://github.com/Ahmedlag/cloadex.git", branch: "main"

  depends_on "go" => :build if build.head?

  def install
    if build.head?
      system "go", "build",
             "-ldflags", "-X github.com/Ahmedlag/cloadex/cmd.version=HEAD-#{Utils.git_short_head}",
             "-o", bin/"cloadex", "."
    else
      bin.install "cloadex"
    end
  end

  def caveats
    <<~EOS
      cloadex requires two external CLI tools to function:

        Claude Code CLI:  brew install claude-code
                    (or)  npm install -g @anthropic-ai/claude-code

        OpenAI Codex CLI: npm install -g @openai/codex

      Both must be authenticated and available in your PATH.

      Getting started:
        cloadex              Start an interactive session (enter prompts one at a time)
        cloadex "<prompt>"   Run a single prompt through the pipeline
        cloadex init         Create a project config file
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/cloadex --version")
    assert_match "Usage:", shell_output("#{bin}/cloadex --help")
  end
end
