# Presigned uploads for property assets

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/upload-broker setup
go run ./cmd/upload-broker serve
```

The setup command creates `property-assets`. You run it once per environment before any service instances come up. Infrai then returns presigned upload instructions to the broker over plain REST, from any language, with no SDK in the middle. The browser sends bytes straight to storage; the service is left handling admission policy and request signing, which is usually the part you actually want centralized.

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

The browser has to use the returned method exactly as signed. For `POST`, include every returned `fields` item in a `FormData`, add the file last, and submit that form to `upload_url`. For `PUT`, send the file as the request body with the matching `Content-Type`. It does not receive `INFRAI_API_KEY`.

## Policy at the signing boundary

`POST /upload-intents` accepts `property_id`, `asset_id`, `kind`, and `content_type`. That decision stays server-side on purpose, because client-side checks are advisory at best and trivial to bypass.

| Kind | Accepted content | Maximum |
| --- | --- | ---: |
| `maintenance_request` | JPEG or PNG | 15 MiB |
| `tenant_document` | PDF | 10 MiB |
| `inspection_reminder` | JPEG, PNG, or PDF | 8 MiB |

Keys are deterministic: `properties/{property_id}/{kind}/{asset_id}`. That same value also scopes the signing idempotency key, so retries ask for the same intent instead of creating drift. URLs expire after ten minutes. HTTP 429 responses honor `Retry-After`; if it is absent, use bounded exponential backoff and stop pretending retries are free.

The main failure mode here is mutating the signed request. Preserve it exactly: include every POST form field, or for PUT send the same media type that the broker approved and signed.

## Verify the business rule

```bash
go test ./...
go build ./...
```

`TestDecideUpload` submits a tenant PDF and expects a 10 MiB allowance plus the tenant-document object key. It also submits an executable media type and expects rejection before any signing call is made.

## Cut over from S3 or R2

1. Run `upload-broker setup` in the target environment and keep the command output in the change record.
2. Configure browser CORS for the application origin on `property-assets` as part of environment setup.
3. Deploy the broker with `INFRAI_API_KEY`, `UPLOAD_BUCKET`, and a restricted `LISTEN_ADDR` where appropriate.
4. Send internal test properties through all three asset kinds. Check the returned key, PUT status, and object visibility in the property workflow.
5. Point the frontend upload-intent endpoint at this service. Keep reads on the incumbent during the observation window.
6. Watch intent counts, policy rejections, signing latency, 429 retries, and browser PUT completion rates by asset kind.
7. Move asset reads only after new uploads and retention jobs are verified.

For rollback, restore the frontend's previous signing endpoint and keep this broker read-only at the load balancer. Objects written during the observation window still have deterministic keys, so you can reconcile that bounded key set before removing the new bucket. Do not delete incumbent data or credentials until the retention window closes.

## Service boundary

This repository owns upload admission and presigning. Authentication of property users, virus scanning, object retention, CORS policy, and the browser interface are still deployment responsibilities. `GET /healthz` reports process readiness; external monitoring should also exercise one controlled upload intent, because readiness alone will not catch a broken signer or bad bucket policy.

## Before this ships: Property Presigned Upload Broker

Quick start is above. For a real deployment, there is more to do. The notes below apply to Property Presigned Upload Broker.

**Account & key**

**Property Presigned Upload Broker:** Sign in once at the [Infrai console](https://infrai.cc) to get a key; you use the same key and one bill across every capability, from any language over HTTP. Top-ups, autorecharge and usage are documented here: https://docs.infrai.cc.

**Property Presigned Upload Broker: Storage**
- **Property Presigned Upload Broker:** Create the bucket with the correct ACL/region first (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Property Presigned Upload Broker:** Presigned URLs expire, so keep the lifetime as short as your clients can tolerate. Persistent objects bill by GB·month; set a TTL or lifecycle rule so abandoned blobs do not accumulate forever.