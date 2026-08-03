#!/bin/bash

CILIUM_START_VERSION="1.19.5"
CILIUM_UPGRADE_VERSION="1.19.6"

echo "Executing test with L7 proxy disabled..."
./test-local.sh "$CILIUM_START_VERSION" "$CILIUM_UPGRADE_VERSION" false

echo "Executing test with L7 proxy enabled..."
./test-local.sh "$CILIUM_START_VERSION" "$CILIUM_UPGRADE_VERSION" true
