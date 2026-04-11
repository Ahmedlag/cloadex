#!/usr/bin/env sh
set -eu

MODULE_PATH="${CLOADEX_MODULE_PATH:-github.com/Ahmedlag/cloadex}"
MODULE_VERSION="${CLOADEX_VERSION:-latest}"

fail() {
  printf 'error: %s\n' "$1" >&2
  exit 1
}

info() {
  printf '%s\n' "$1"
}

require_go() {
  if ! command -v go >/dev/null 2>&1; then
    fail "Go is required. Install Go first, then rerun this script."
  fi
}

go_bin_dir() {
  gobin="$(go env GOBIN 2>/dev/null || true)"
  if [ -n "$gobin" ]; then
    printf '%s\n' "$gobin"
    return
  fi

  gopath="$(go env GOPATH 2>/dev/null || true)"
  if [ -n "$gopath" ]; then
    printf '%s/bin\n' "$gopath"
    return
  fi

  printf '%s/go/bin\n' "$HOME"
}

display_path() {
  target="$1"
  case "$target" in
    "$HOME"/*)
      printf '$HOME/%s\n' "${target#"$HOME"/}"
      ;;
    *)
      printf '%s\n' "$target"
      ;;
  esac
}

profile_path() {
  if [ -n "${PROFILE:-}" ]; then
    printf '%s\n' "$PROFILE"
    return
  fi

  shell_name="$(basename "${SHELL:-}")"
  case "$shell_name" in
    zsh)
      printf '%s\n' "${ZDOTDIR:-$HOME}/.zshrc"
      ;;
    bash)
      if [ -f "$HOME/.bashrc" ]; then
        printf '%s\n' "$HOME/.bashrc"
      elif [ -f "$HOME/.bash_profile" ]; then
        printf '%s\n' "$HOME/.bash_profile"
      else
        printf '%s\n' "$HOME/.bashrc"
      fi
      ;;
    ksh)
      printf '%s\n' "$HOME/.kshrc"
      ;;
    fish)
      printf '\n'
      ;;
    *)
      printf '%s\n' "$HOME/.profile"
      ;;
  esac
}

path_contains() {
  target="$1"
  case ":$PATH:" in
    *":$target:"*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

ensure_path() {
  target="$1"

  if path_contains "$target"; then
    info "PATH already contains $target"
    return
  fi

  profile="$(profile_path)"
  rendered_target="$(display_path "$target")"
  line="export PATH=\"\$PATH:$rendered_target\""

  if [ -z "$profile" ]; then
    info "Shell profile auto-detection is not supported for your shell."
    info "Add this line yourself:"
    info "  $line"
    return
  fi

  if [ ! -f "$profile" ]; then
    : >"$profile"
  fi

  if grep -F "$line" "$profile" >/dev/null 2>&1; then
    info "PATH entry already present in $profile"
    return
  fi

  {
    printf '\n# Added by cloadex installer\n'
    printf '%s\n' "$line"
  } >>"$profile"

  info "Added $target to PATH in $profile"
  info "Run 'source $profile' or open a new shell before using cloadex."
}

main() {
  require_go

  bin_dir="$(go_bin_dir)"
  mkdir -p "$bin_dir"

  info "Installing $MODULE_PATH@$MODULE_VERSION ..."
  go install "$MODULE_PATH@$MODULE_VERSION"

  ensure_path "$bin_dir"

  if [ -x "$bin_dir/cloadex" ]; then
    info "Installed cloadex to $bin_dir/cloadex"
  fi

  if command -v cloadex >/dev/null 2>&1; then
    info "cloadex is ready: $(command -v cloadex)"
  else
    info "cloadex was installed, but your current shell may need PATH reloaded first."
  fi
}

main "$@"
