# OP-stack itest

To run the e2e tests, first you need to set up the devnet data:

```bash
$ make op-e2e-devnet
```

Then run the following command to start the e2e tests:

```bash
$ make test-e2e-op

# Run all tests
$ make test-e2e-op

# Filter specific test
$ make test-e2e-op-filter FILTER=TestFinalityGadget
```
