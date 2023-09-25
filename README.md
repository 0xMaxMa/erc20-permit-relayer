# 🔑 ERC20 Permit Relayer RPC
The Relayer RPC is designed as a proxy service, acting as an intermediary between users and an endpoint RPC, with support for the ERC20Permit contract to store pending transactions from users. It facilitates relaying and batch sending of these transactions to the blockchain.

The service provides users with additional information, such as `ERC20.balanceOf()`, allowing them to instantly access unrealized balances without waiting for unnecessary transaction confirmations on-chain. Furthermore, users can send the `delegate_permit` command by sending a signed permit signature to delegate transactions on-chain with gasless.

## Core Components
- `ProcessRequest`: This component processes various methods, validates permit signatures, and stores transactions in the pending queue or forwards the request to the endpoint RPC.

- `Signer`: The Signer is responsible for signing the transaction from ProcessRequest and interval sending a batch of transactions from the pending queue to the blockchain.

- `Keeper`: This component sync processes transactions, monitors finalized transactions, and clears them from the pending queue.

## Architecture Design
![Relayer's Architecture](https://github.com/0xMaxMa/erc20-permit-relayer/blob/main/docs/design.png)

## Local Development
Require go version >= 1.19

Git clone: `git clone https://github.com/0xMaxMa/erc20-permit-relayer.git`

Prerequirement:
1. Postgres database server or start local with docker compose: `docker compose up -d`
1. Generate account keystore with unlock password for sign transactions, [read more](https://geth.ethereum.org/docs/getting-started#generating-accounts)

Local Run:
1. Configure in: `config.go`
1. Run Relayer: `go run .`

Building the source:

1. Build: `make relayer` 
1. Run Relayer: `./build/bin/relayer`

## Related
ERC20Permit Contract: [https://github.com/0xMaxMa/digital10k-contracts.git](https://github.com/0xMaxMa/digital10k-contracts.git)
