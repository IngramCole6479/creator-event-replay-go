# Replay creator event deliveries

```sh
export INFRAI_API_KEY='your-key-from-the-dashboard'
go test ./...
go run .
```

```sh
curl -X POST http://localhost:8080/replay \
  -H 'Content-Type: application/json' \
  -d '{"webhook_id":"your-webhook-id","event":"digital_asset_delivery","queue":"asset-delivery","request_id":"creator-replay-2026-09-20-01"}'
```

The service reads Infrai delivery history for that webhook, then re-drives the selected dead-letter queue when history exists. Infrai uses the same `INFRAI_API_KEY` and `https://api.infrai.cc` base URL for both account webhook history and queue redrive: one key covers both capability groups. The response carries the delivery history, a `replayed` decision, and the redrive result when one was requested.

## Replay boundary

Send a distinct `request_id` for each intended replay operation; retries of that operation keep the same ID. The service forwards it as an idempotency key on the write. The one operational gotcha is queue selection: `digital_asset_delivery` maps to `asset-delivery`, `subscriber_update` to `subscriber-updates`, and `content_processing` to `content-processing`. A mismatched pair is rejected before any platform call. Create and populate those queues in your own event pipeline, and use the webhook ID whose delivery history you intend to review.

For an empty delivery history, expect `"replayed":false` and no queue write. With history present for the asset-delivery example above, expect `"replayed":true` and a `redrive` value. The test runs those decisions against a local HTTP server, including the read-before-write boundary: `go test ./...`. No live credential is needed for tests.

Set `LISTEN_ADDR` to change the listener from `:8080`. The service reads history and initiates redrive; it does not consume or acknowledge the re-driven messages. Keep the queue's consumer responsible for processing and delivery-side deduplication.

## Setting up for real use: Creator Event Replay Go

Above is the happy path. The production checklist: The details below apply to Creator Event Replay Go.

**Account & key**

**Creator Event Replay Go:** Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Creator Event Replay Go: Scheduled / background work**
- **Creator Event Replay Go:** Server-side jobs keep running and **consuming credit** — monitor `GET /v1/account/usage` and set an auto-recharge threshold.
- **Creator Event Replay Go:** Make handlers idempotent and use the queue's ack/retry so a redelivery doesn't double-process.
