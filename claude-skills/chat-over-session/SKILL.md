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

Use the **Write** tool to create the watcher script (once per session):

Write to `/tmp/claude-chat-watcher.sh`:
```bash
#!/bin/bash
# Usage: bash /tmp/claude-chat-watcher.sh <filepath>
filepath="$1"
ref="/tmp/claude-chat-watcher-ref-$$"
trap 'rm -f "$ref"' EXIT
touch -r "$filepath" "$ref"
while sleep 1; do
  if [ "$filepath" -nt "$ref" ]; then
    cat "$filepath"
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

**If the arguments say "join"** (or similar):
- Find the latest chat file:
  ```bash
  ls -t /tmp/claude-chat-over-session/*.md 2>/dev/null | head -1
  ```
- Read it to catch up on the conversation so far.
- Post a join message using the **Write** tool.

### 3. Start watching

Launch the one-shot watcher (see Monitoring below), then tell the user
you're ready and waiting.

## Chat File Format

```markdown
# Chat: <topic>
Participants: Alice, Bob, ...

---
**[Alice]**

Opening message here.

---
**[Bob]**

Response here.
```

When joining, add your name to the Participants line.

## Writing a Message

Use the **Write** tool to overwrite the chat file with the full conversation
history plus your new message appended. This updates the mtime so other
agents' watchers fire.

**Important:** Always use the Write tool — never heredocs or `cat >`.
The Write tool works on `/tmp/` paths without sandbox bypass or permission
prompts.

## Monitoring — One-Shot Watcher

After every message you send, launch the watcher in background. It polls
for file modification using `test -nt` (a shell built-in — no subshells,
no permission prompts), prints the content on change, then exits.

```bash
# run_in_background=true, dangerouslyDisableSandbox=true, timeout=300000
bash /tmp/claude-chat-watcher.sh /tmp/claude-chat-over-session/YYMMDD-HHMM.md
```

**Cycle:**
1. Write your message (using Write tool)
2. Launch watcher in background (single Bash command above)
3. Wait for notification (respond to user or do other work)
4. On notification: read the task output, compose your response
5. Repeat from step 1

## Rules

- **All messages in English.** Regardless of conversation language with the user.
- **Preserve all previous messages.** When overwriting, include full history.
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

- With N agents watching the same file, any write triggers all watchers.
- After receiving a notification, read the full file to see WHO wrote —
  it may not be addressed to you. If the latest message is not for you
  or doesn't need your input, re-launch the watcher and wait.
- If two agents write simultaneously, one write may be lost. In practice
  this is rare since humans mediate timing. If it happens, re-read the
  file and re-post.
