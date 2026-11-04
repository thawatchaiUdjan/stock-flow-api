# 0009: Product images in Supabase Storage, uploaded directly by the browser

Status: Accepted
Date: 2026-11-04

## Context

Products will have images. Passing files through the Go API would spend function time and request-size limits on bytes the API never needs to read.

## Decision

Images live in a public-read Supabase Storage bucket. The API issues a signed upload URL and the browser uploads to it directly. The API never handles the file. Reads go straight to the bucket's public URL.

The bucket, and the code that reads `SUPABASE_URL`, `SUPABASE_SECRET_KEY` and `SUPABASE_STORAGE_BUCKET`, arrive with the first feature that stores images. Until then `.env.example` doesn't list them, because it lists only what the code reads.

## Consequences

- The secret key stays server-side, and the browser only ever holds a short-lived upload URL.
- Images are public to anyone with the URL. Fine for product photos; anything private needs a different bucket.

## Alternatives considered

- Uploading through the API: more function time and request-size limits, for no benefit.
- Vercel Blob: a second storage vendor, when Supabase already hosts the database.
