# formflow (Go)

[![CI](https://github.com/formizeflow/formflow-go/actions/workflows/ci.yml/badge.svg)](https://github.com/formizeflow/formflow-go/actions/workflows/ci.yml)

Small client for the FormFlow REST API. Standard library only, Go 1.21+ (generics). Tested in CI on Go 1.21 and stable.

```sh
go get github.com/formizeflow/formflow-go
```

```go
ff, _ := formflow.New(formflow.Options{APIKey: os.Getenv("FORMFLOW_API_KEY"), BaseURL: "https://forms.example.com"})
ctx := context.Background()

form, err := ff.CreateForm(ctx, formflow.CreateFormInput{Name: "Feedback"})
_, err = ff.PublishForm(ctx, form.ID)

err = ff.EachSubmission(ctx, form.ID, url.Values{"status": {"completed"}}, func(s formflow.Submission) error {
	fmt.Println(s.ID, s.Answers)
	return nil
})
```

- **Auth**: bearer key; `WorkspaceID` sets `FormFlow-Workspace`; `Version` pins the API version.
- **Retries**: 429, 5xx and network errors, `MaxRetries` (default 3, `-1` for none), exponential backoff with jitter, `Retry-After` honored. A cancelled context is never retried.
- **Idempotency**: every POST carries an `Idempotency-Key`, generated if you leave `Request.IdempotencyKey` empty and reused across retries.
- **Errors**: `*formflow.Error` with `Status`, `Code`, `Message`, `Details`, `RequestID`, `RetryAfter`.
- **Pagination**: `formflow.Each[T]` / `formflow.List[T]` work on any list endpoint.
- **Everything else**: `formflow.Data[T](ctx, ff, formflow.Request{Method: "POST", Path: "/approvals/apr_1/cancel", Body: map[string]any{}})`.

## Webhooks

```go
body, _ := io.ReadAll(r.Body) // raw bytes, not re-encoded JSON
ev, err := formflow.ConstructEvent(secret, body, r.Header.Get("FormFlow-Signature"))
```

Scheme: `FormFlow-Signature: t=<unix>,v1=<hex HMAC-SHA256(secret, "t.body")>`, 5 minute window, several `v1` values accepted during secret rotation. The test suite checks the signature against a vector computed independently with Node's `crypto`.

## Tests

`go test ./...` in this directory.
