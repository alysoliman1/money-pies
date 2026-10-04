# money-pies

## Config Directory

The default location for this project's main config directory is `$HOME/.money-pies`.
Below is an example of the config directory's structure.
```bash
.money-pies
└── schwab
    ├── client-config.json
    └── client-token.json
```
Each supported brokerage (right now only schwab) will have its own subdirectory,
with at least two json files **client-config.json** and **client-token.json**.
The **client-config.json** file stores all information needed for connecting to
the brokerage API through OAuth2.0 (i.e. **client-id**, **client-secret**, etc).
The **client-token.json** stores the access token retrieved at the end of an OAuth2.0 flow. 

## Authenticating

Go to `./cmd/schwab-oauth` and run `sh run.sh`.

## MCP Server

`./cmd/mcp-server` serves a brokerage's trading account over the
[Model Context Protocol](https://modelcontextprotocol.io) (stdio transport).
The tools are defined in `./internal/pkg/mcpserver` and wrap the read-only part of the
`TradingAccount` interface: the server cannot place or cancel orders. Every tool call
loads the account from the brokerage again, so results are never served from a cache.

| Tool | TradingAccount methods |
|------|------------------------|
| `get_account_summary` | `Type`, `TotalCash`, `CashAvailableForTrading`, `CashAvailableForWithdrawal`, `LongMarketValue`, `ShortMarketValue`, `PendingDeposits` |
| `get_positions` | `Positions` |
| `get_latest_prices` | `LatestRegularMarketPrices` |
| `get_order_status` | `GetOrderStatus` |
| `get_recent_orders` | `GetRecentOrders` |

Environment variables:

- `BROKERAGE`: `schwab` or `alpaca` (required)
- `MONEY_PIES_CONFIG`: config directory (defaults to `$HOME/.money-pies`)
- `SCHWAB_ACCOUNT_NUMBER`: account number to read (required for schwab)

Example registration with Claude Code:
```bash
go build -o bin/mcp-server ./cmd/mcp-server
claude mcp add money-pies -e BROKERAGE=schwab -e SCHWAB_ACCOUNT_NUMBER=<account-number> -- $PWD/bin/mcp-server
```
