#!/bin/sh
set -eu

# These names are fixed; file values are never printed or passed in argv.
suma_load_secret() {
  suma_secret_name="$1"
  suma_secret_file="$2"
  [ -n "$suma_secret_file" ] || return 0
  if [ ! -r "$suma_secret_file" ]; then
    printf '%s\n' "Required secret file is not readable: $suma_secret_name" >&2
    exit 1
  fi
  suma_secret_value=$(cat -- "$suma_secret_file")
  if [ -z "$suma_secret_value" ]; then
    printf '%s\n' "Required secret file is empty: $suma_secret_name" >&2
    exit 1
  fi
  export "$suma_secret_name=$suma_secret_value"
  unset suma_secret_value
}

suma_load_secret SUMA_SETUP_TOKEN "${SUMA_SETUP_TOKEN_FILE:-}"
suma_load_secret SUMA_DB_DSN "${SUMA_DB_DSN_FILE:-}"
suma_load_secret SUMA_INGRESS_MASTER_KEY "${SUMA_INGRESS_MASTER_KEY_FILE:-}"
suma_load_secret SUMA_RUNNER_TOKEN "${SUMA_RUNNER_TOKEN_FILE:-}"
suma_load_secret SUMA_KNOWLEDGE_TOKEN "${SUMA_KNOWLEDGE_TOKEN_FILE:-}"
suma_load_secret SUMA_KNOWLEDGE_OB_URL "${SUMA_KNOWLEDGE_OB_URL_FILE:-}"
suma_load_secret SUMA_EMBEDDING_API_KEY "${SUMA_EMBEDDING_API_KEY_FILE:-}"
exec "$@"
