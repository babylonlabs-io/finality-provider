#!/bin/bash
go test -run FuzzCreateFinalityProvider -fuzz=FuzzCreateFinalityProvider -fuzztime=30s -v > test_output.txt 2>&1
if grep -q "FAIL" test_output.txt; then
    exit 1  # Test failed, flaky behavior
else
    exit 0  # Test passed, no flakiness
fi