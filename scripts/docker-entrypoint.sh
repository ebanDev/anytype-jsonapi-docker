#!/bin/sh
set -e

: "${DATA_ROOT:=/data}"
: "${JSONAPI_ADDR:=0.0.0.0:31009}"
: "${GRPC_ADDR:=0.0.0.0:31007}"
: "${GRPCWEB_ADDR:=0.0.0.0:31008}"
: "${APP_NAME:=jsonapi-cli}"
: "${PLATFORM:=jsonapi-cli}"
: "${VERSION:=0.0.0-headless}"
: "${TIMEOUT:=3m}"
: "${WAIT_SPACES:=3m}"

if [ -z "${MNEMONIC}" ]; then
  echo "MNEMONIC env var is required" >&2
  exit 1
fi

mkdir -p "${DATA_ROOT}"

# Start the gRPC server (JsonAPI will bind via bootstrap)
./grpcserver "${GRPC_ADDR}" "${GRPCWEB_ADDR}" &
SERVER_PID=$!

stop() {
  kill "${SERVER_PID}" 2>/dev/null || true
}
trap stop INT TERM

# Give the server a moment to bind
sleep 2

# Bootstrap: recover/select account, enable JsonAPI, wait for spaces
./jsonapi_bootstrap \
  -root "${DATA_ROOT}" \
  -mnemonic "${MNEMONIC}" \
  -jsonapi "${JSONAPI_ADDR}" \
  -app-name "${APP_NAME}" \
  -platform "${PLATFORM}" \
  -version "${VERSION}" \
  -timeout "${TIMEOUT}" \
  -wait-spaces "${WAIT_SPACES}"

wait "${SERVER_PID}"
