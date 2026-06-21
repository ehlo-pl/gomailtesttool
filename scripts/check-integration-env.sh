#!/bin/sh
# check-integration-env.sh — Validate required MSGRAPH* env vars before running integration tests.
# Called by: make integration-test
# Exits 1 with a clear error message if any required variable is missing.

set -e

REQUIRED_VARS="MSGRAPHTENANTID MSGRAPHCLIENTID MSGRAPHSECRET MSGRAPHMAILBOX"
MISSING=""

for var in $REQUIRED_VARS; do
    eval "val=\$$var"
    if [ -z "$val" ]; then
        MISSING="$MISSING $var"
    fi
done

if [ -n "$MISSING" ]; then
    echo ""
    echo "ERROR: Missing required environment variable(s) for integration tests:"
    for var in $MISSING; do
        echo "  - $var"
    done
    echo ""
    echo "Set them before running 'make integration-test', for example:"
    echo "  export MSGRAPHTENANTID=your-tenant-id"
    echo "  export MSGRAPHCLIENTID=your-client-id"
    echo "  export MSGRAPHSECRET=your-client-secret"
    echo "  export MSGRAPHMAILBOX=test@example.com"
    echo ""
    echo "Or use the built-in env helper:"
    echo "  gomailtest devtools env set"
    echo ""
    exit 1
fi

echo "All required MSGRAPH* environment variables are set."
