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

Go to `./apps/schwab-oauth` and run `sh run.sh`.

## MCP Server

`./apps/mcp-server` serves a brokerage's trading account over the
[Model Context Protocol](https://modelcontextprotocol.io) (stdio transport).
The tools are defined in `./internal/pkg/mcpserver` and wrap the `TradingAccount` interface.
`place_order` and `cancel_order` act on the real account: an order is sent to the brokerage as soon as
the tool is called. The account is created once
at startup; `get_account_summary` calls `RefreshAccount` before reading the balances, and the
other tools query the brokerage directly on every call, so nothing is served from a cache.

| Tool | TradingAccount methods |
|------|------------------------|
| `get_account_summary` | `RefreshAccount`, `Type`, `TotalCash`, `CashAvailableForTrading`, `CashAvailableForWithdrawal`, `LongMarketValue`, `ShortMarketValue`, `PendingDeposits` |
| `get_positions` | `Positions` |
| `get_latest_prices` | `LatestRegularMarketPrices` |
| `get_order_status` | `GetOrderStatus` |
| `get_recent_orders` | `GetRecentOrders` |
| `backtrack` | `Positions`, `DailyClosingPrices` (through `internal/pkg/backtracker`) |
| `place_order` | `PlaceOrder` |
| `cancel_order` | `CancelPendingOrder` |

Environment variables:

- `BROKERAGE`: `schwab` or `alpaca` (required)
- `MONEY_PIES_CONFIG`: config directory (defaults to `$HOME/.money-pies`)
- `SCHWAB_ACCOUNT_NUMBER`: account number to trade with (required for schwab)

Example registration with Claude Code:
```bash
go build -o bin/mcp-server ./apps/mcp-server
claude mcp add money-pies -e BROKERAGE=schwab -e SCHWAB_ACCOUNT_NUMBER=<account-number> -- $PWD/bin/mcp-server
```

## Account Viewer

`./apps/account-viewer` serves a local web page that visualizes the connected trading account:
balances, the largest positions, the biggest unrealized gains and losses, every position in a
sortable table, and recent orders.

```bash
sh apps/account-viewer/schwab.sh
```

It opens `http://127.0.0.1:8090` in the browser. Pass `-addr 127.0.0.1:<port>` to use another port
and `-no-open` to skip opening the browser. It takes the same environment variables as the MCP server.

The page also has a **Backtrack** tool: it simulates what $10,000 would have become if it had been held
in today's positions, at today's weights, over the last 1, 3 or 5 years, with optional rebalancing.
It runs on `internal/pkg/backtracker`, which backtracks any `Pie` against a source of daily closing prices.
The result is price return only (no dividends, fees or taxes), and positions without price history at
the start of the period are left out and listed.

The viewer is read-only. It holds the account as a `ReadOnlyTradingAccount`, which has no methods for
placing or cancelling orders, and it only answers `GET` requests. The page has no login, so the server
only listens on a loopback address and rejects requests addressed to any other host.
