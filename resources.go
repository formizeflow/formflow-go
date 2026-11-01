package formflow

import (
	"context"
	"net/http"
	"net/url"
)

// Form is the subset of fields most callers need; Draft is the editable definition left as raw JSON.
type Form struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspaceId"`
	Name          string         `json:"name"`
	Slug          string         `json:"slug"`
	Status        string         `json:"status"`
	Draft         map[string]any `json:"draft"`
	DraftRevision int            `json:"draftRevision"`
	CreatedAt     int64          `json:"createdAt"`
	UpdatedAt     int64          `json:"updatedAt"`
}

type Submission struct {
	ID            string            `json:"id"`
	FormID        string            `json:"formId"`
	Status        string            `json:"status"`
	Answers       map[string]any    `json:"answers"`
	Computed      map[string]any    `json:"computed"`
	PaymentStatus string            `json:"paymentStatus"`
	CreatedAt     int64             `json:"createdAt"`
	CompletedAt   *int64            `json:"completedAt"`
	Metadata      map[string]any    `json:"metadata"`
	Attribution   map[string]string `json:"attribution"`
}

type CreateFormInput struct {
	Name       string         `json:"name"`
	Definition map[string]any `json:"definition,omitempty"`
	TemplateID string         `json:"templateId,omitempty"`
}

func (c *Client) ListForms(ctx context.Context, q url.Values) (Page[Form], error) {
	return List[Form](ctx, c, "/forms", q)
}
func (c *Client) GetForm(ctx context.Context, id string) (Form, error) {
	return Data[Form](ctx, c, Request{Method: http.MethodGet, Path: "/forms/" + id})
}
func (c *Client) CreateForm(ctx context.Context, in CreateFormInput) (Form, error) {
	return Data[Form](ctx, c, Request{Method: http.MethodPost, Path: "/forms", Body: in})
}

// PublishForm fails with *Error{Code: "validation_failed"} and Details["issues"] when the draft has errors.
func (c *Client) PublishForm(ctx context.Context, id string) (map[string]any, error) {
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/forms/" + id + "/publish", Body: map[string]any{}})
}

// EditForm applies draft operations atomically. expectedRevision 0 skips the conflict check.
func (c *Client) EditForm(ctx context.Context, id string, ops []map[string]any, expectedRevision int) (map[string]any, error) {
	body := map[string]any{"operations": ops}
	if expectedRevision > 0 {
		body["expectedRevision"] = expectedRevision
	}
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/forms/" + id + "/operations", Body: body})
}

func (c *Client) ListSubmissions(ctx context.Context, formID string, q url.Values) (Page[Submission], error) {
	return List[Submission](ctx, c, "/forms/"+formID+"/submissions", q)
}

// EachSubmission walks every matching response across pages.
func (c *Client) EachSubmission(ctx context.Context, formID string, q url.Values, fn func(Submission) error) error {
	return Each(ctx, c, "/forms/"+formID+"/submissions", q, fn)
}

func (c *Client) GetSubmission(ctx context.Context, id string) (Submission, error) {
	return Data[Submission](ctx, c, Request{Method: http.MethodGet, Path: "/submissions/" + id})
}

// BulkTriage sets status, assignee or tags on up to 500 responses. Keys: status, assigneeId, addTags, removeTags.
func (c *Client) BulkTriage(ctx context.Context, ids []string, patch map[string]any) (map[string]any, error) {
	body := map[string]any{"ids": ids}
	for k, v := range patch {
		body[k] = v
	}
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/submissions/bulk-triage", Body: body})
}

// Refund refunds a paid one-time payment. confirmAmountMinor must equal the amount refunded
// (the remaining balance when amountMinor is 0).
func (c *Client) Refund(ctx context.Context, paymentID string, confirmAmountMinor, amountMinor int64) (map[string]any, error) {
	body := map[string]any{"confirmAmountMinor": confirmAmountMinor}
	if amountMinor > 0 {
		body["amountMinor"] = amountMinor
	}
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/payments/" + paymentID + "/refund", Body: body})
}
