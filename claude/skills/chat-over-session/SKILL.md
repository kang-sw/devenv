---
name: chat-over-session
description: >
  Multi-agent chat across Claude Code sessions. Agents discuss topics via a
  shared file.
argument-hint: 'you are "AgentName". <topic or "join the discussion">'
---

# Chat Over Session

$ARGUMENTS

## Setup

Parse your agent name from the arguments (the quoted string after "you are").
Then determine whether to **create** or **join** a chat.

### 1. Ensure chat directory and watcher script exist

First check if script file exists in path `/tmp/claude-chat-watcher.sh`, read and
use the **Write** tool to create the watcher script (once per session) if file 
does not exist or the content mismatches with following code snippet:

```bash
#!/bin/bash
# Usage: bash /tmp/claude-chat-watcher.sh <filepath> <fence>
# <fence> = line count the agent has already seen.
# Waits until the file has more lines than <fence>, then outputs
# only the new lines and prints the updated fence on the last line.
filepath="$1"
fence="${2:-0}"
while sleep 1; do
  current=$(wc -l < "$filepath")
  if [ "$current" -gt "$fence" ]; then
    tail -n +"$((fence + 1))" "$filepath"
    echo "__FENCE:$current"
    break
  fi
done
```

Then ensure the chat directory exists:
```bash
mkdir -p /tmp/claude-chat-over-session  # dangerouslyDisableSandbox=true
```

### 2. Create or join

**If the arguments contain a discussion topic** (not just "join"):
- Get a timestamp:
  ```bash
  date +"%y%m%d-%H%M"  # → e.g. 260314-1530
  ```
- Use the **Write** tool to create `/tmp/claude-chat-over-session/YYMMDD-HHMM.md`
  with the header and your opening message (see Chat File Format below).
- Set your initial fence to the line count of what you just wrote.

**If the arguments say "join"** (or similar):
- Find the latest chat file:
  ```bash
  ls -t /tmp/claude-chat-over-session/*.md 2>/dev/null | head -1
  ```
- Read it to catch up on the conversation so far.
- Set your initial fence to the line count of what you just read.
- Post a join message (see Writing a Message below).

### 3. Start watching

Launch the one-shot watcher with your current fence (see Monitoring
below), then tell the user you're ready and waiting.

## Chat File Format

The chat file is **append-only**. It starts with a header, and each message
is appended as a new block.

```markdown
# Chat: <topic>

---
**[Alice]**

Opening message here.

---
**[Bob]**

Response here.
```

## Writing a Message — Append via Buffer

Never read and rewrite the full chat file. Instead, use a **buffer file**
that gets appended:

1. **Write** your message to a buffer file using the Write tool:
   `/tmp/claude-chat-over-session/YYMMDD-HHMM-YourName.md`

   Buffer content (always include the separator):
   ```markdown

   ---
   **[YourName]**

   Your message here.
   ```

2. **Append** the buffer to the chat file and remove it:
   ```bash
   # dangerouslyDisableSandbox=true
   cat /tmp/claude-chat-over-session/YYMMDD-HHMM-YourName.md >> /tmp/claude-chat-over-session/YYMMDD-HHMM.md && rm /tmp/claude-chat-over-session/YYMMDD-HHMM-YourName.md
   ```

3. **Update your fence** by adding the number of lines in your message
   to your current fence value.

This keeps context usage constant — you only hold your new message in
context, never the full history.

## Monitoring — Fence-Based Watcher

Each agent tracks a **fence** — the line count it has already seen.
The watcher polls until the file has more lines than the fence, then
outputs only the new lines.

```bash
# run_in_background=true, dangerouslyDisableSandbox=true, timeout=300000
bash /tmp/claude-chat-watcher.sh /tmp/claude-chat-over-session/YYMMDD-HHMM.md <fence>
```

Replace `<fence>` with your current fence value (a number).

**Why this handles races:** If agent B and C both respond to agent A,
and you only saw A's message, your fence is still at A's position.
The next watcher immediately returns both B's and C's messages because
the line count jumped past your fence. No message is ever lost.

The last line of watcher output is `__FENCE:<n>` — parse this to get
your updated fence for the next watcher launch.

**Cycle:**
1. Write your message to buffer (Write tool)
2. Append buffer to chat file (single Bash command)
3. Update your fence (add your message's line count)
4. Launch watcher with your fence in background
5. Wait for notification
6. On notification: read the task output — it contains only unseen
   messages plus `__FENCE:<n>` on the last line
7. Update your fence from the `__FENCE` value
8. Repeat from step 1

## Rules

- **All messages in English.** Regardless of conversation language with the user.
- **Append-only.** Never overwrite the chat file. Always append via buffer.
- **Track your fence.** Always pass your current fence to the watcher.
- **One message at a time.** Write, then wait for a response.
- **Identify yourself.** Every message block starts with `**[YourName]**`.
- **Stay on topic.** Use the original topic as anchor. Note topic shifts explicitly.
- **Signal completion.** When consensus is reached, write `#done` and a summary
  of agreed outcomes / action items.
- **Don't monopolize.** Keep messages focused. If multiple agents are present,
  address specific agents when relevant.
- **Report to user.** After each exchange, briefly tell the user what was discussed
  and any decisions made. The user cannot see the chat file directly.

## Multi-Agent Considerations

- With N agents watching the same file, any append triggers all watchers
  whose fence is behind the new line count.
- After receiving new messages, check WHO wrote. If the latest message
  is not addressed to you or doesn't need your input, update your fence
  and re-launch the watcher without responding.
- If two agents append simultaneously, both writes succeed (append is
  atomic for small writes). Your fence-based watcher catches both
  regardless of timing — it compares line counts, not timestamps.

## Doctrine

Chat-over-session bridges independent Claude Code sessions through a
shared append-only file. Every design choice optimizes for **message
integrity under concurrency**: append-only writes prevent corruption,
fence-based monitoring ensures no message is missed regardless of
timing, and buffer-based appending keeps each agent's context constant.
When a rule is ambiguous, apply whichever interpretation better preserves
reliable message delivery across concurrent sessions.
