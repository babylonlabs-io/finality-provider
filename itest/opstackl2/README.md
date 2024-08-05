# OP-stack itest

To run the e2e tests, first you need to set up the devnet data:

```bash
$ make op-e2e-devnet
```

Next, replace `babylonFinalityGadgetBitcoinRpc` in `itest/opstackl2/devnet-data/devnetL1.json` with your own Bitcoin mainnet RPC URL to avoid potential rate limit issues.

Then run the following command to start the e2e tests:

```bash
$ make test-e2e-op

# Filter specific test
$ make test-e2e-op Filter=TestFinalityGadget
```
