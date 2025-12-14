# Anytype Heart JsonAPI Docker

Containerized, headless build of [`anyproto/anytype-heart`](https://github.com/anyproto/anytype-heart) with the JsonAPI enabled. The image builds the upstream `grpcserver`, runs a small bootstrap helper, and only exposes the JsonAPI port (`31009`).

## Quick start
- Create a `.env` file beside `docker-compose.yml` with your recovery phrase (do not commit it):
  ```
  MNEMONIC="word1 word2 ... word12"
  ```
- Build and start: `docker compose up --build -d`
- Follow logs to see the generated bearer token and JsonAPI address: `docker compose logs -f anytype-heart-jsonapi`
- Call the API with the printed token, e.g.:
  ```
  curl -H "Authorization: Bearer <token>" http://localhost:31009/v1/spaces
  ```

## Data and ports
- Account data is stored in `./data` (mounted to `/data` inside the container). Keep this directory to preserve your account and caches between restarts.
- Only the JsonAPI port is published (`31009:31009`); gRPC (`31007`) and gRPC-Web (`31008`) stay inside the container to reduce the attack surface.

## Security notes
- Anytype Heart is not designed to be broadly exposed on the public internet. Put this service behind a reverse proxy with proper authentication, rate limiting, and TLS.
- Keep your `.env` and `./data` directory private; the bearer token and mnemonic grant full access to the account.

## Environment variables
The entrypoint enforces `MNEMONIC` and accepts these optional overrides (defaults shown):
- `DATA_ROOT=/data` – where account data lives (mapped to `./data`)
- `JSONAPI_ADDR=0.0.0.0:31009` – JsonAPI listen address
- `GRPC_ADDR=0.0.0.0:31007`, `GRPCWEB_ADDR=0.0.0.0:31008` – internal listeners
- `APP_NAME=jsonapi-cli`, `PLATFORM=jsonapi-cli`, `VERSION=0.0.0-headless` – labels used when creating the app key/session
- `TIMEOUT=3m`, `WAIT_SPACES=3m` – per-RPC timeout and how long to wait for spaces to sync before listing

On startup the entrypoint launches the gRPC server, recovers the wallet from `MNEMONIC`, enables the JsonAPI, generates an app key, waits for spaces to sync, and prints the token you can use with the API.
