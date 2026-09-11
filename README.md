# flymap

An MCP (Model Context Protocol) server that exposes your local Protonmail Bridge as 12 tools for an LLM. Built with the [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk), [go-imap/v2](https://github.com/emersion/go-imap) for IMAP, and [enmime](https://github.com/jhillyerd/enmime) for MIME parsing.

## Prerequisites

- **Go 1.23+**
- **Protonmail Bridge** running locally (default: IMAP `127.0.0.1:1143`, SMTP `127.0.0.1:1025`, STARTTLS)

## Build

```sh
go build -o flymap
```

## Configuration

All configuration is via environment variables with the `FLYMAP_` prefix:

| Variable | Default | Description |
|---|---|---|
| `FLYMAP_IMAP_HOST` | `127.0.0.1` | Protonmail Bridge IMAP host |
| `FLYMAP_IMAP_PORT` | `1143` | Bridge IMAP port |
| `FLYMAP_SMTP_HOST` | `127.0.0.1` | Protonmail Bridge SMTP host |
| `FLYMAP_SMTP_PORT` | `1025` | Bridge SMTP port |
| `FLYMAP_USERNAME` | *(required)* | Full email address |
| `FLYMAP_PASSWORD` | *(required)* | Bridge password (not your Proton login password) |
| `FLYMAP_TLS_MODE` | `starttls` | TLS mode: `starttls`, `ssl`, or `none` |
| `FLYMAP_FROM_ADDRESS` | = username | From address for sent mail |

TLS certificate verification is skipped (`InsecureSkipVerify: true`) since Bridge uses a self-signed certificate and traffic stays on localhost.

## Running

### Stdio (default — for Claude Desktop, etc.)

```sh
FLYMAP_USERNAME=you@protonmail.com FLYMAP_PASSWORD=your-bridge-password ./flymap
```

### HTTP (optional)

```sh
FLYMAP_USERNAME=you@protonmail.com FLYMAP_PASSWORD=your-bridge-password ./flymap --http :8080
```

## MCP Client Configuration

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "flymap": {
      "command": "/path/to/flymap",
      "env": {
        "FLYMAP_USERNAME": "you@protonmail.com",
        "FLYMAP_PASSWORD": "your-bridge-password"
      }
    }
  }
}
```

## Tools

### Reading Mail

| Tool | Description |
|---|---|
| `list_folders` | List all mailbox folders with total and unread message counts |
| `list_messages` | List recent messages (envelopes only). Params: `folder` (default INBOX), `limit` (default 50, max 200), `unread_only` |
| `search_messages` | Search by keyword across headers and body. Params: `query` (required), `folder`, `limit` (default 20, max 100) |
| `get_message` | Get full message with body, headers, and attachments. Params: `uid` (required), `folder`. Body text truncated at 50KB. |

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

- **Per-operation IMAP connections**: Each tool invocation dials Bridge fresh (dial → STARTTLS → login → operate → logout). Localhost latency makes this practical and avoids connection state management.
- **MIME parsing via enmime**: Raw RFC822 from IMAP is parsed with `enmime.ReadEnvelope()` for clean access to text, HTML, and attachment metadata.
- **SMTP via stdlib**: `net/smtp` with STARTTLS and PLAIN auth, matching Bridge's SMTP expectations.

## License

MIT
