#!/bin/sh
# Runs the read-only account viewer against the Schwab trading account.
# Authenticate first with apps/schwab-oauth/run.sh.
export MONEY_PIES_CONFIG="${MONEY_PIES_CONFIG:-$HOME/.money-pies}"
export BROKERAGE=schwab
export SCHWAB_ACCOUNT_NUMBER="${SCHWAB_ACCOUNT_NUMBER:-30650409}"
cd "$(dirname "$0")" && exec go run . "$@"
