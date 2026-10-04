## TradingAccount Interface

TradingAccount is an interface that all trading account integrations must implement.
It provides the following methods.

| Method | Description |
|--------|-------------|
| `LatestRegularMarketPrices(ctx, symbols)` | Retrieves the latest regular market prices for the given symbols. |
| `TotalCash(ctx)` | Retrieves the total cash amount in the trading account. |
| `CashAvailableForTrading(ctx)` | Retrieves the cash available for trading in the trading account. |
| `CashAvailableForWithdrawal(ctx)` | Retrieves the cash available for withdrawal in the trading account. |
| `LongMarketValue(ctx)` | Retrieves the long market value in the trading account. |
| `ShortMarketValue(ctx)` | Retrieves the short market value in the trading account. |
| `PendingDeposits(ctx)` | Retrieves the pending deposits in the trading account. |
| `Type(ctx)` | Retrieves the type of the trading account. |
| `Positions(ctx)` | Retrieves the current positions for the account. |
| `PlaceOrder(ctx, order)` | Places a new order for the account. |
| `GetOrderStatus(ctx, orderID)` | Retrieves the status of a specific order. |
| `CancelPendingOrder(ctx, orderID)` | Cancels a pending order. |
| `GetRecentOrders(ctx, limit)` | Retrieves recent orders for the account. |

## Pie

## Investor 

The TradingAccount interface is injected in the Investor object. The Investor object
executes any complex or custom logic for making trade orders. Any logic in the investor object
can therefore run for any brokerage integration that implements the TradingAccount interface.

### PlacePieOrderWithoutFractionalShares
This is the first instance of complex logic supported by the Investor object.
#### Use Case
We're given a set of $n$ symbols and each symbol has price $c_i$ and weight $w_i$ such that $\sum_{i=1}^nw_i=1$. We're given a total amount to invest $I$ and ideally we want to invest $Iw_i$ in each symbol. However, our brokerage of choice does not allow fractional share trading for the given set of shares, and so we want to make trades that approximate the given weight distribution while maximizing the amount invested from the original amount $I$. Furthermore, we each symbol may have a pre-invested amount $p_i$ from before that we can account for.
#### Implementation
For each symbol, we need to invest $\max(Iw_i-p_i,0)$ but since we can't have fractional shares the amount actually needed would be
$\lfloor\max(Iw_i-p_i,0)/c_i\rfloor c_i$. Hence, if we invest $x_i$ in each symbol then the vector $x=(x_1,\ldots,x_n)$ needs to satisfy the constraints $x_i\geq \lfloor\max(Iw_i-p_i,0)/c_i\rfloor c_i$ and $\sum_ix_i\leq I$. The problem is then to maximize $\sum_ix_i$ under these constraints, which is a very simple linear program.


Since shares are whole, each $x_i$ must also be a multiple of $c_i$, so the implementation works in two steps:
1. Buy the minimum $\lfloor\max(Iw_i-p_i,0)/c_i\rfloor$ shares of each symbol.
2. Spend what is left of $I$ one share at a time. Each time, among the symbols whose share price still fits in the remaining amount, buy the one that ends up the least above its target $Iw_i$ (ties go to the symbol that appears first in the pie). Stop when no share fits.

The amount left uninvested is therefore always less than the cheapest share price. Step 2 is greedy: it favors staying close to the weights over finding the combination of shares that leaves the absolute smallest remainder.

All orders are market buy orders. Nothing is ordered if the amount, the pie, the pre-invested amounts or the prices are invalid, or if the amount exceeds the cash available for trading. If the brokerage rejects an order, the remaining orders are still placed and the failures are returned together.
