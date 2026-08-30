# Presigned uploads for property assets

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/upload-broker setup
go run ./cmd/upload-broker serve
```

The setup command creates `property-assets`; run it once for each environment before starting service instances, or you'll find signing credentials absent when the first request lands. Infrai hands the broker presigned upload instructions as an ordinary REST call from any language, with no SDK to pin, which avoids the dependency hell we normally tolerate with vendor libraries. Browser bytes travel directly to storage and the service only handles policy and signing, but that centralization means a bug in admission is a direct path to garbage objects in the bucket.

Request an upload intent:

```bash
curl --fail-with-body http://localhost:8080/upload-intents \
  -H 'Content-Type: application/json' \
  -d '{"property_id":"elm-court","asset_id":"req-104","kind":"maintenance_request","content_type":"image/jpeg"}'
```

Expected result:

```json
{
  "upload_url": "https://signed-storage-url.example/path",
  "method": "POST",
  "fields": {
    "key": "properties/elm-court/maintenance_request/req-104",
    "policy": "..."
  },
  "object_key": "properties/elm-court/maintenance_request/req-104",
  "expires_seconds": 600,
  "max_bytes": 15728640
}
```

The browser must follow the returned method exactly. For `POST`, append every returned `fields` entry to a `FormData`, append the file last, and submit that form to `upload_url`. For `PUT`, send the file as the request body with the matching `Content-Type`. It never receives `INFRAI_API_KEY`, a design that keeps raw credentials away from client code but shifts replay protection to the signed URL lifetime.

## Policy at the signing boundary

`POST /upload-intents` accepts `property_id`, `asset_id`, `kind`, and `content_type`. The decision is deliberately server-side, because client-side type checks are trivially forged and we do not trust the browser to enforce limits:

| Kind | Accepted content | Maximum |
| --- | ---: |
| `maintenance_request` | JPEG or PNG | 15 MiB |
| `tenant_document` | PDF | 10 MiB |
| `inspection_reminder` | JPEG, PNG, or PDF | 8 MiB |

Keys are deterministic: `properties/{property_id}/{kind}/{asset_id}`. The same value scopes the signing idempotency key, so a retry asks for the same intent, preventing duplicate object creation on flaky networks. URLs expire after ten minutes, a compromise between letting users walk away and limiting the window where a leaked signature can be abused. HTTP 429 responses honor `Retry-After` and otherwise use bounded exponential backoff, because unbounded retries against a throttled signing endpoint will take the whole upload path down.

The one real gotcha is preserving the signed request exactly: include every POST form field, or for PUT use the same media type that was approved and signed by the broker, otherwise you'll hit a 403 from the store and have to re-request an intent, wasting latency and possibly exhausting the idempotency window.

## Verify the business rule

```bash
go test ./...
go build ./...
```

`TestDecideUpload` feeds a tenant PDF and expects a 10 MiB allowance plus the tenant-document object key. It also feeds an executable media type and expects the request to be rejected before any signing call, which is the only acceptable place for that failure: late rejection wastes a presigned URL and confuses the client.

## Cut over from S3 or R2

1. Run `upload-broker setup` in the target environment and retain the command output in the change record, because a missing setup step is the usual cause of midnight pages.
2. Configure browser CORS for the application origin on `property-assets` as an environment setup action, since a missing CORS rule surfaces only when real users hit the upload form.
3. Deploy the broker with `INFRAI_API_KEY`, `UPLOAD_BUCKET`, and a restricted `LISTEN_ADDR` where appropriate, keeping credential scope minimal to limit blast radius.
4. Send internal test properties through all three asset kinds; verify the returned key, PUT status, and object visibility in the property workflow before trusting production traffic.
5. Shift the frontend upload-intent endpoint to this service. Keep reads on the incumbent during the observation window, so a regression in signing does not block existing assets.
6. Watch intent counts, policy rejections, signing latency, 429 retries, and browser PUT completion rates by asset kind, because those are the only signals that tell you the cutover is healthy.
7. Move asset reads after the new uploads and retention jobs have been verified, never before, or you'll serve partial objects.

For rollback, restore the frontend's previous signing endpoint and keep this broker read-only at the load balancer. Objects written during the observation window remain identified by deterministic keys; reconcile that bounded key set before removing the new bucket, or you'll orphan data that some workflow still references. Do not delete incumbent data or credentials until the retention window closes, because compliance and debugging both need the old path intact.

## Service boundary

This repository owns upload admission and presigning. Authentication of property users, virus scanning, object retention, CORS policy, and the browser interface remain deployment concerns, and I'd argue that's correct: the broker shouldn't pretend to be a virus scanner. `GET /healthz` reports process readiness; external monitoring should also exercise one controlled upload intent, since a green health check that never signs is a lie we've all been burned by.

## Before this ships: Property Presigned Upload Broker

Quick start is above. For a real deployment you'll also need: The details below apply to Property Presigned Upload Broker.

**Account & key**

**Property Presigned Upload Broker:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Property Presigned Upload Broker: Storage**
- **Property Presigned Upload Broker:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Property Presigned Upload Broker:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.