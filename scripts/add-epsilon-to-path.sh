#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
BIN_DIR="$REPO_ROOT/bin"
PATH_LINE="export PATH=\"$BIN_DIR:\$PATH\""

if [ ! -d "$BIN_DIR" ]; then
	mkdir -p "$BIN_DIR"
fi

case ":$PATH:" in
	*":$BIN_DIR:"*)
		echo "$BIN_DIR is already on PATH for this shell."
		exit 0
		;;
esac

if [ -n "${SHELL:-}" ]; then
	case "$(basename -- "$SHELL")" in
		zsh)
			PROFILE="$HOME/.zshrc"
			;;
		bash)
			if [ "$(uname -s)" = "Darwin" ]; then
				PROFILE="$HOME/.bash_profile"
			else
				PROFILE="$HOME/.bashrc"
			fi
			;;
		*)
			PROFILE="$HOME/.profile"
			;;
	esac
else
	PROFILE="$HOME/.profile"
fi

if [ -f "$PROFILE" ] && grep -F "$BIN_DIR" "$PROFILE" >/dev/null 2>&1; then
	echo "$BIN_DIR is already configured in $PROFILE."
else
	{
		printf '\n# Add local Epsilon builds to PATH\n'
		printf '%s\n' "$PATH_LINE"
	} >> "$PROFILE"
	echo "Added $BIN_DIR to PATH in $PROFILE."
fi

echo "Run this command to update your current shell:"
echo ". \"$PROFILE\""
