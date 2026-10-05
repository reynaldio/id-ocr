# Handoff — Google Vision API key leaks through error text (v0.2.0)

- **From:** dossio (fix note `docs/fix-notes/2026-10-05-vision-api-key-in-ocr-errors.md`
  in the dossio repo); also found by shima.
- **Severity:** Medium for a library — every consumer that logs, reports or returns
  `Recognize` errors can expose the caller's Google Cloud API key.
- **Consumers already protected on their side:** dossio (`pkg/ocrclient` wraps every
  engine and returns a fixed error; branch `fix/ocr-key-leak`) and shima. The library
  should still be fixed so new consumers are safe by default.

## Problem

`ocr/googlevision/googlevision.go`:

1. **The key travels in the URL.** `newHTTPRequest` (line ~135) builds
   `…/images:annotate?key=<KEY>`.
2. **A transport failure returns that URL.** `Recognize` (line ~107) returns
   `fmt.Errorf("googlevision: request failed: %w", err)`. `err` is a `*url.Error`,
   whose `Error()` is `Post "https://vision.googleapis.com/v1/images:annotate?key=<KEY>": dial tcp …`.
   So a DNS blip, timeout or refused connection puts the full key in the error text.
3. **An HTTP failure returns the provider body.** Line ~116 returns
   `googlevision: HTTP %d: <body, up to 512 bytes>`. Google's body is usually a
   structured error without the key, but a proxy or gateway in between can echo the
   request URL, and the body is untrusted text either way.

A URL query key also tends to land in proxy and load-balancer access logs.

## Fix

1. **Send the key as a header, not a query parameter.** Google's REST APIs accept
   `X-Goog-Api-Key: <KEY>`. In `newHTTPRequest`, drop the `?key=` and set
   `req.Header.Set("X-Goog-Api-Key", e.apiKey)` when `apiKey != ""` (keep the existing
   `Authorization: Bearer` path for tokens). The URL then never contains a secret, so
   point 2 can no longer leak it even if someone wraps `*url.Error` later.
2. **Don't wrap `*url.Error` as is.** Return the cause without the URL, e.g.
   `var ue *url.Error; if errors.As(err, &ue) { err = ue.Err }` before wrapping, and keep
   `%w` so callers can still detect `context.DeadlineExceeded` / `context.Canceled`.
3. **Don't put the raw HTTP body in the error.** Parse Google's error envelope
   (`{"error":{"code":403,"message":"…","status":"PERMISSION_DENIED"}}`) and return
   `googlevision: HTTP 403 PERMISSION_DENIED`. Fall back to the status code alone when
   the body doesn't parse. The existing per-response `error.message` handling
   (`TestRecognize_BearerTokenAndAPIError`) can stay — that body comes from Vision itself
   and doesn't echo the request.

## Tests to add / change (`ocr/googlevision/googlevision_test.go`)

- `TestRecognize_ParsesTextAndWords` asserts `r.URL.Query().Get("key") == "secret"` —
  change it to assert the `X-Goog-Api-Key` header **and** that `r.URL.RawQuery` is empty.
- New: an `httptest` server that answers 403 with a body echoing `r.URL.String()` and the
  `X-Goog-Api-Key` header → the error mentions 403 but contains neither the key nor the
  echoed text.
- New: an unreachable endpoint (start an `httptest.Server`, take its URL, `Close()` it) →
  the error does not contain the key and `errors.Is(err, context.DeadlineExceeded)` still
  works for a timed-out ctx.

## Release

- Core module only (`ocr/googlevision` is in the core module); `ocr/tesseract` is
  unaffected. Tag **v0.2.1**. Update the README if it shows the `?key=` form.
- Then in dossio: `go get github.com/reynaldio/id-ocr@v0.2.1` in `code/apps/server`, run
  the OCR tests in Docker. Keep dossio's own guard in `pkg/ocrclient` — it's
  defence in depth and also hides provider error bodies from users.

## Resolution (2026-10-06, v0.2.1)

Implemented as above, plus:

- Package doc comment updated (it said "API key (query parameter)").
- The default HTTP client refuses redirects: Go forwards custom headers such as
  `X-Goog-Api-Key` to a cross-host redirect target. A client passed through
  `WithHTTPClient` keeps its own redirect policy (documented on the option).
- Transport and read errors also go through a redaction pass that replaces the API key
  and bearer token with `[REDACTED]`. The result still unwraps.
- The `status` from the error envelope is only included when it is UPPER_SNAKE_CASE, so
  a proxy can't sneak free text through it.
- The timeout test uses a handler that blocks, not a closed server. A closed server
  refuses the connection at once and never hits the deadline.
- Following CLAUDE.md, the Tesseract submodule's core `require` was bumped too. Tag both
  `v0.2.1` and `ocr/tesseract/v0.2.1`.
