
# Code style
All http calls must be made with the go-rest package found here https://pkg.go.dev/github.com/go-resty/resty/v2


# Workflow
- Make sure to use the commands in Makefile instead of raw go cli commands
- Be sure to check your changes with 'make lint' command


# Schwab MCP server
- `cmd/mcp-server/schwab.sh` sets the env variables for Schwab and runs the MCP server. It exits at startup if the Schwab token is missing or can no longer be refreshed.
- When asked to "auth the mcp" (or the MCP server fails with a token/authentication error), start the Schwab OAuth flow:
  1. Run `sh cmd/schwab-oauth/run.sh` in the background. It opens the Schwab login page in the browser and listens on `https://127.0.0.1:8080` for the redirect.
  2. Tell the user to log in and approve access in the browser, and to accept the browser's warning about the self-signed certificate when Schwab redirects back.
  3. Wait for the command to print `OAuth2.0 flow complete` (or `already authenticated`). The token is saved to the `token_file` set in `~/.money-pies/schwab/client-config.json`.
  4. Ask the user to reconnect the MCP server with `/mcp`.
- Never print the contents of `client-config.json` or `client-token.json`.
