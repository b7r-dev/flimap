# Contributing to flimap

Pull requests are welcome — whether AI-created or not. We review what we can but may not be able to respond to each one.

## Bug Reports

Open a [GitHub Issue](../../issues) with:
- What you expected to happen
- What actually happened
- Your IMAP/SMTP provider, OS, and Go version
- Relevant env vars (redact passwords)

## Pull Requests

1. Fork the repo and create a branch from `main`
2. Keep changes focused — one concern per PR
3. Make sure it builds and passes vet:
   ```sh
   go build ./...
   go vet ./...
   ```
4. If adding a new tool, include a description in the tool registration and update the README tool table
5. Don't commit `connection-details.txt`, the built binary, or `.DS_Store`

## Code Style

- Follow standard Go conventions (`gofmt`, `goimports`)
- Errors should wrap with context: `fmt.Errorf("doing X: %w", err)`
- Logging goes to stderr via `log/slog` — stdout is reserved for the MCP stdio protocol

## Project Structure

| File | Responsibility |
|---|---|
| `main.go` | Entry point, CLI flags, MCP server setup |
| `mail.go` | Config, IMAP connection management, virtual folder helpers |
| `imap.go` | All IMAP operations (folders, messages, search, mutations) |
| `smtp.go` | SMTP message composition and sending |
| `tools.go` | MCP tool handler definitions and registration |

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
