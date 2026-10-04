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
