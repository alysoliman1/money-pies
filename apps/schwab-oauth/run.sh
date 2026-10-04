export SCHWAB_CLIENT_CONFIG="${SCHWAB_CLIENT_CONFIG:-$HOME/.money-pies/schwab/client-config.json}"
# The local certificate is loaded relative to this directory.
cd "$(dirname "$0")" && go run .
