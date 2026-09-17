# flimap

An MCP (Model Context Protocol) server that exposes your local email (IMAP/SMTP) as 12 tools for an LLM agent. Built for [Protonmail Bridge](https://proton.me/mail/bridge) but works with any standard IMAP/SMTP server.

Email is the last locked-up data silo for most people. flimap lets your AI agent read, search, organize, and send email — all running locally, no cloud, no third-party API.

Built with the [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk), [go-imap/v2](https://github.com/emersion/go-imap) for IMAP, and [enmime](https://github.com/jhillyerd/enmime) for MIME parsing.

## Quick Start

```sh
# Install
go install github.com/b7r-dev/flimap@latest

# Set credentials
export FLIMAP_USERNAME=you@protonmail.com
export FLIMAP_PASSWORD=your-bridge-password

# Run
flimap
```

Then add it to your MCP client config (see below).

## Prerequisites

- **Go 1.23+** (or download a prebuilt binary from [Releases](../../releases))
- **An IMAP/SMTP server** accessible from localhost

### Protonmail Bridge (default)

Install [Protonmail Bridge](https://proton.me/mail/bridge), add your account, and note the Bridge password from the app. Defaults are IMAP `127.0.0.1:1143`, SMTP `127.0.0.1:1025`, STARTTLS.

### Other providers

flimap works with any standard IMAP/SMTP server. Just change the host/port/TLS env vars:

| Provider | IMAP Host:Port | SMTP Host:Port | TLS Mode |
|---|---|---|---|
| Protonmail Bridge | `127.0.0.1:1143` | `127.0.0.1:1025` | `starttls` |
| Gmail | `imap.gmail.com:993` | `smtp.gmail.com:587` | `ssl` / `starttls` |
| Fastmail | `imap.fastmail.com:993` | `smtp.fastmail.com:465` | `ssl` |
| Self-hosted (Dovecot/Postfix) | your host:143/993 | your host:25/587/465 | varies |

For Gmail, use an [App Password](https://myaccount.google.com/apppasswords), not your account password.

## Build from Source

```sh
git clone https://github.com/b7r-dev/flimap.git
cd flimap
go build -o flimap
```

## Configuration

All configuration is via environment variables with the `FLIMAP_` prefix:

| Variable | Default | Description |
|---|---|---|
| `FLIMAP_IMAP_HOST` | `127.0.0.1` | IMAP server host |
| `FLIMAP_IMAP_PORT` | `1143` | IMAP server port |
| `FLIMAP_SMTP_HOST` | `127.0.0.1` | SMTP server host |
| `FLIMAP_SMTP_PORT` | `1025` | SMTP server port |
| `FLIMAP_USERNAME` | *(required)* | Email address (login + From header) |
| `FLIMAP_PASSWORD` | *(required)* | Mail password (Bridge password, app password, etc.) |
| `FLIMAP_TLS_MODE` | `starttls` | TLS mode: `starttls`, `ssl`, or `none` |
| `FLIMAP_FROM_ADDRESS` | = username | Override From address for sent mail |

**Security:** TLS certificate verification is skipped by default (`InsecureSkipVerify: true`) since Bridge uses a self-signed certificate and traffic stays on localhost. If you're connecting to a remote server with a real certificate, you may want to change this in the source. All credentials stay local — flimap makes no network calls beyond your IMAP/SMTP server. Note that any MCP client config you paste credentials into stores them in plaintext on disk — see the warning under [MCP Client Configuration](#mcp-client-configuration).

## Running

### Stdio (default — for Claude Desktop, OpenCode, etc.)

```sh
FLIMAP_USERNAME=you@protonmail.com FLIMAP_PASSWORD=your-bridge-password flimap
```

### HTTP (optional)

```sh
FLIMAP_USERNAME=you@protonmail.com FLIMAP_PASSWORD=your-bridge-password flimap --http :8080
```

## MCP Client Configuration

> **Credentials in plaintext:** the examples below put your mail password directly into the client's config file, where it sits in plaintext on disk. Treat that file as a secret — don't commit or share it, keep it user-readable only (`chmod 600`), and assume it rides along in backups and support exports. A Protonmail Bridge password only unlocks IMAP/SMTP on localhost, so the stakes are modest; an app password for a remote provider (Gmail, Fastmail) is a full mail credential. Since flimap reads its config from environment variables, you can keep the password out of the config file entirely — see the notes under each client below.

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "flimap": {
      "command": "/path/to/flimap",
      "env": {
        "FLIMAP_USERNAME": "you@protonmail.com",
        "FLIMAP_PASSWORD": "your-bridge-password"
      }
    }
  }
}
```

Claude Desktop passes `env` values through literally — it has no variable expansion — so the password has to live in this file. To keep it out, point `command` at a small wrapper script that supplies it from the macOS Keychain instead:

```sh
#!/bin/sh
# one-time setup: security add-generic-password -s flimap -a you@protonmail.com -w
exec env FLIMAP_PASSWORD="$(security find-generic-password -s flimap -w)" /path/to/flimap
```

Save it as e.g. `/path/to/flimap-wrapper`, make it executable, set `"command": "/path/to/flimap-wrapper"`, and drop the `FLIMAP_PASSWORD` line from the `env` block.

### OpenCode

`opencode.json` supports variable substitution (`{env:VAR}`, `{file:path}`), so the password doesn't need to appear in the config. Export `FLIMAP_PASSWORD` in your shell profile, or store it in a file only you can read, and reference it:

```json
{
  "mcp": {
    "servers": {
      "flimap": {
        "type": "local",
        "command": ["/path/to/flimap"],
        "environment": {
          "FLIMAP_USERNAME": "you@protonmail.com",
          "FLIMAP_PASSWORD": "{env:FLIMAP_PASSWORD}"
        }
      }
    }
  }
}
```

`{file:~/.secrets/flimap-password}` works too. Note that `{env:...}` resolves to an empty string when the variable isn't set, in which case flimap exits with a clear `FLIMAP_PASSWORD is required` error. See OpenCode's [variable substitution docs](https://opencode.ai/docs/config/#variables).

## Tools

### Reading Mail

| Tool | Description |
|---|---|
| `list_folders` | List all mailbox folders recursively with total/unread counts, hierarchy metadata, and `\Noselect` container detection |
| `list_messages` | List recent messages (envelopes only). Params: `folder` (default INBOX), `limit` (default 50, max 200), `unread_only` |
| `search_messages` | Search by keyword across headers and body. Params: `query` (required), `folder`, `limit` (default 20, max 100) |
| `get_message` | Get full message with body, HTML, headers, and attachment list. Params: `uid` (required), `folder`. Body text truncated at 50KB. |

### Sending Mail

| Tool | Description |
|---|---|
| `send_message` | Compose and send email via SMTP. Params: `to[]` (required), `subject` (required), `body` (required), `cc[]`, `bcc[]` |

### Organizing Mail

| Tool | Description |
|---|---|
| `move_message` | Move message between folders. Params: `uid`, `folder`, `dest_folder` |
| `mark_read` | Set `\Seen` flag. Params: `uid`, `folder` |
| `mark_unread` | Remove `\Seen` flag. Params: `uid`, `folder` |
| `delete_message` | Delete message — moves to Trash, or permanently deletes if already in Trash. Params: `uid`, `folder` |
| `create_folder` | Create a new mailbox. Params: `name` |
| `rename_folder` | Rename a mailbox. Params: `old_name`, `new_name` |
| `delete_folder` | Delete a mailbox. Params: `name` |

## Architecture

- **Per-operation IMAP connections**: Each tool invocation dials fresh (dial → STARTTLS → login → operate → logout). Localhost latency makes this practical and avoids connection state management.
- **MIME parsing via enmime**: Raw RFC822 from IMAP is parsed with `enmime.ReadEnvelope()` for clean access to text, HTML, and attachment metadata.
- **SMTP via stdlib**: `net/smtp` with STARTTLS and PLAIN auth.
- **Hierarchical folders**: Folders are listed recursively with `/` delimiters. Container folders (`\Noselect`) are marked as non-selectable — only their children can hold messages.
- **Virtual folders**: Programmatic support for address-based virtual folders that filter All Mail by `To:` header. Retained in the codebase for custom configurations.

## License

MIT — Copyright (c) 2026 Aggressively Beige Holdings, LLC
