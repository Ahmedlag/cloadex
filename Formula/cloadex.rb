class Cloadex < Formula
  desc "CLI that orchestrates Claude Code and OpenAI Codex into a collaborative coding workflow"
  homepage "https://github.com/Ahmedlag/cloadex"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/Ahmedlag/cloadex/releases/download/v#{version}/cloadex-darwin-arm64.tar.gz"
      sha256 "f5ded17b6d07772d9ec99344d560c16a4547483b18fa87112dd7c681fd22404b" # Run `make formula` to fill after release build
    end
    on_intel do
      url "https://github.com/Ahmedlag/cloadex/releases/download/v#{version}/cloadex-darwin-amd64.tar.gz"
      sha256 "9f9212e4c9d2adb21fc03b348167434ab3bd2a961f7c7c3e22aa48da8d27e8ab" # Run `make formula` to fill after release build
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/Ahmedlag/cloadex/releases/download/v#{version}/cloadex-linux-arm64.tar.gz"
      sha256 "58eaf3f68fb654f41367e2e711884aa6b9e5faa7f40876b66224c80f05972f96" # Run `make formula` to fill after release build
    end
    on_intel do
      url "https://github.com/Ahmedlag/cloadex/releases/download/v#{version}/cloadex-linux-amd64.tar.gz"
      sha256 "8f8c2d24fe38a6409d308591d6f035bf6dfb110fbad7daebaf9f91b4708d8662" # Run `make formula` to fill after release build
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
