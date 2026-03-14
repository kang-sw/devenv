---
name: chat-over-session
description: >
  Multi-agent chat across Claude Code sessions. Agents discuss topics via a
  shared file. Use: /chat-over-session you are "Name". discuss X with others.
argument-hint: 'you are "AgentName". <topic or "join the discussion">'
---

# Chat Over Session

$ARGUMENTS

## Setup

Parse your agent name from the arguments (the quoted string after "you are").
Then determine whether to **create** or **join** a chat.

### 1. Ensure chat directory exists

```bash
mkdir -p /tmp/claude-chat-over-session  # dangerouslyDisableSandbox=true
```

### 2. Create or join

**If the arguments contain a discussion topic** (not just "join"):
- Create a new chat file with timestamp:
  ```bash
  date +"%y%m%d-%H%M"  # → e.g. 260314-1530
  ```
- Write the initial file at `/tmp/claude-chat-over-session/YYMMDD-HHMM.md`
  with the header and your opening message.

**If the arguments say "join"** (or similar):
- Find the latest chat file:
  ```bash
  ls -t /tmp/claude-chat-over-session/*.md 2>/dev/null | head -1
  ```
- Read it to catch up on the conversation so far.
- Post a join message.

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

Read the current file content, append your new message, and overwrite
the file entirely. This updates the mtime so other agents' watchers fire.

```bash
# Read current content, append, overwrite
cat > /tmp/claude-chat-over-session/<file>.md << 'EOF'
<all previous content>

---
**[YourName]**

Your message here.
EOF
```

Use `dangerouslyDisableSandbox=true` for all file operations in `/tmp/`.

## Monitoring — One-Shot Watcher

Launch this in background after every message you send. It polls for file
modification, prints the content on change, then exits — triggering a
background task notification.

```bash
# run_in_background=true, dangerouslyDisableSandbox=true
# Cross-platform: macOS uses `stat -f %m`, Linux uses `stat -c %Y`
_mtime() { stat -f %m "$1" 2>/dev/null || stat -c %Y "$1" 2>/dev/null || echo 0; }
last_mod=$(_mtime <filepath>)
while sleep 1; do
  cur_mod=$(_mtime <filepath>)
  [ "$cur_mod" != "$last_mod" ] && { cat <filepath>; break; }
done
```

**Cycle:**
1. Write your message (overwrite file)
2. Launch watcher in background
3. Wait for notification (respond to user or do other work)
4. On notification: read the output, compose your response
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
