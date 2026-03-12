#!/bin/bash
# Navigate to adjacent window and select the pane closest to where we came from.
# Usage: tmux-cross-window.sh <arrive-from>
#   arrive-from: "left" = we came from the left  → select leftmost pane
#                "right" = we came from the right → select rightmost pane

dir="$1"

if [ "$dir" = "right" ]; then
  # Going left (came from right) → previous window, select rightmost pane
  tmux previous-window
  while [ "$(tmux display-message -p '#{pane_at_right}')" != "1" ]; do
    tmux select-pane -R
  done
elif [ "$dir" = "left" ]; then
  # Going right (came from left) → next window, select leftmost pane
  tmux next-window
  while [ "$(tmux display-message -p '#{pane_at_left}')" != "1" ]; do
    tmux select-pane -L
  done
fi
