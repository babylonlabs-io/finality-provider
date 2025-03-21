#!/usr/bin/env bash

set -eo pipefail

cd proto
buf dep update
buf generate .
cd ..

go mod tidy -compat=1.20
