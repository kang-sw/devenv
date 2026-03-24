#!/usr/bin/env bash
# Custom wrapper around tmux-fzf command.sh
# Appends a trailing space so the cursor isn't glued to the command name.

FZF_DEFAULT_OPTS="$FZF_DEFAULT_OPTS --header='Select a command.'"
TMUX_FZF_DIR="$HOME/.tmux/plugins/tmux-fzf/scripts"
source "$TMUX_FZF_DIR/.envs"

target_origin=$(tmux list-commands)
target=$(printf "[cancel]\n%s" "$target_origin" | eval "$TMUX_FZF_BIN $TMUX_FZF_OPTIONS" | cut -d ' ' -f 1)

[[ "$target" == "[cancel]" || -z "$target" ]] && exit
tmux command-prompt -I "$target "
