# RainX Chain

RainX Chain is an independent native blockchain implementation for **RainX Coin (RXC)**. It does not use Ethereum, BNB Chain, Solana, Polygon, or an ERC-20/SPL contract.

This project is intentionally isolated from the existing RainX app RXC ledger. The app can be connected later through a controlled migration/bridge layer.

## Protocol v1

- Native asset: RainX Coin (RXC)
- Chain ID: `rainx-mainnet-1`
- Address: Base58Check, version `0x52`
- Signatures: Ed25519
- Hashing: SHA-256
- Ledger: UTXO
- Consensus: Proof-of-Work
- Target block time: 60 seconds
- Initial block reward: 50 RXC
- Halving interval: 210,000 blocks
- Maximum supply target: 21,000,000 RXC
- Monetary precision: 8 decimal places
- Default network ports: P2P `27777`, RPC `27778`

## What is included

- deterministic transaction encoding and IDs
- Ed25519 wallet generation/signing/verification
- Base58Check addresses
- UTXO validation and double-spend checks
- block headers, Merkle roots and proof-of-work
- chain validation and reorg selection by cumulative work
- durable block/mempool storage using the standard library
- TCP P2P peer protocol
- JSON-RPC/HTTP API
- mobile-facing address UTXO/history/block endpoints
- permissive CORS for native clients during development
- miner
- wallet encryption using PBKDF2-HMAC-SHA256 + AES-256-GCM
- browser explorer/wallet dashboard
- unit tests for cryptography, transactions, UTXO rules and PoW

## Important launch rule

A functioning implementation is not automatically a safe public-money mainnet. Before real funds are used, the network needs multi-node interoperability tests, fuzzing, adversarial testing, economic review, replay protection review, crash/recovery testing, long-running synchronization tests and an independent security audit.

## Run

```bash
go test ./...
go run ./cmd/rainxd node --data ./data --rpc 127.0.0.1:27778 --p2p 0.0.0.0:27777
```

In another terminal:

```bash
go run ./cmd/rainxd wallet create --data ./data --name main --password 'change-me'
go run ./cmd/rainxd wallet address --data ./data --name main --password 'change-me'
go run ./cmd/rainxd mine --data ./data --rpc http://127.0.0.1:27778 --address RXC_ADDRESS
```

The dashboard is available at `http://127.0.0.1:27778/`.

## Mobile wallet

The companion `rainx-wallet` project is a native React Native / Expo application for Android and iOS. It can create/restore a RainX wallet, keep the signing key in device secure storage, sign protocol-compatible transactions locally, read UTXOs/history, and submit signed transactions to this node.

## Production warning

Run the chain as a development network until multi-node interoperability, peer discovery/synchronization, difficulty retargeting, fork/reorg behavior, mempool conflict handling, crash recovery, fuzzing and an independent security audit are complete.
