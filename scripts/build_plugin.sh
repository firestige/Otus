#!/bin/bash

set -e

PLUGIN_TYPE=$1
PLUGIN_NAME=$2

if [ -z "$PLUGIN_TYPE" ] || [ -z "$PLUGIN_NAME" ]; then
    echo "Usage: $0 <plugin_type> <plugin_name>"
    echo "Example: $0 gatherer pcap"
    exit 1
fi

PLUGIN_DIR="plugins/${PLUGIN_TYPE}/${PLUGIN_NAME}"
BUILD_DIR="build/plugins/${PLUGIN_TYPE}"

if [ ! -d "$PLUGIN_DIR" ]; then
    echo "Error: Plugin directory '$PLUGIN_DIR' does not exist."
    exit 1
fi

mkdir -p "$BUILD_DIR"

echo "Building plugin '$PLUGIN_NAME' of type '$PLUGIN_TYPE'..."
cd "$PLUGIN_DIR"

go build -buildmode=plugin -o "../../../$BUILD_DIR/${PLUGIN_NAME}.so" .

if [ $? -eq 0 ]; then
    echo "Successfully built plugin '$PLUGIN_NAME'."
else
    echo "Error: Failed to build plugin '$PLUGIN_NAME'."
    exit 1
fi