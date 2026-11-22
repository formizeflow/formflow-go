package formflow

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"time"
)

type Member struct {
	MembershipID string `json:"membershipId"`
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	Role         string `json:"role"`
}

type APIKey struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Scopes     []string `json:"scopes"`
	Key        string   `json:"key"` // only set by CreateAPIKey, shown once
	ExpiresAt  *int64   `json:"expiresAt"`
	LastUsedAt *int64   `json:"lastUsedAt"`
	RevokedAt  *int64   `json:"revokedAt"`
	CreatedAt  int64    `json:"createdAt"`
}

// AuditEvent ActorName is the user's name or email, the API key's name, "System", or nil when unknown.
type AuditEvent struct {
	ID           string         `json:"id"`
	ActorType    string         `json:"actorType"`
	ActorID      string         `json:"actorId"`
	ActorName    *string        `json:"actorName"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   *string        `json:"resourceId"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    int64          `json:"createdAt"`
}

// Export is the job returned by every export endpoint; URL is a short-lived download link once Status is "done".
type Export struct {
	Export struct {
		ID     string  `json:"id"`
		Format string  `json:"format"`
		Status string  `json:"status"`
		Error  *string `json:"error"`
	} `json:"export"`
	URL string `json:"url"`
}

type Table struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Columns []map[string]any `json:"columns"`
}

func (c *Client) ListMembers(ctx context.Context) ([]Member, error) {
	return Data[[]Member](ctx, c, Request{Method: http.MethodGet, Path: "/members"})
}
func (c *Client) InviteMember(ctx context.Context, email, role string) (Member, error) {
	return Data[Member](ctx, c, Request{Method: http.MethodPost, Path: "/members", Body: map[string]any{"email": email, "role": role}})
}
func (c *Client) SetMemberRole(ctx context.Context, membershipID, role string) (Member, error) {
	return Data[Member](ctx, c, Request{Method: http.MethodPatch, Path: "/members/" + membershipID, Body: map[string]any{"role": role}})
}
func (c *Client) RemoveMember(ctx context.Context, membershipID string) error {
	return c.Do(ctx, Request{Method: http.MethodDelete, Path: "/members/" + membershipID}, nil)
}

func (c *Client) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	return Data[[]APIKey](ctx, c, Request{Method: http.MethodGet, Path: "/api-keys"})
}

// CreateAPIKey returns the full key once. expiresAt is epoch milliseconds, 0 for none.
func (c *Client) CreateAPIKey(ctx context.Context, name string, scopes []string, expiresAt int64) (APIKey, error) {
	body := map[string]any{"name": name, "scopes": scopes}
	if expiresAt > 0 {
		body["expiresAt"] = expiresAt
	}
	return Data[APIKey](ctx, c, Request{Method: http.MethodPost, Path: "/api-keys", Body: body})
}
func (c *Client) RevokeAPIKey(ctx context.Context, id string) error {
	return c.Do(ctx, Request{Method: http.MethodDelete, Path: "/api-keys/" + id}, nil)
}

// ListAuditEvents filters: action, resourceType, from, to (epoch ms or ISO 8601), cursor, limit.
func (c *Client) ListAuditEvents(ctx context.Context, q url.Values) (Page[AuditEvent], error) {
	return List[AuditEvent](ctx, c, "/audit-events", q)
}

// ListOrgAuditEvents spans every workspace of the organization. Org owners and admins only.
func (c *Client) ListOrgAuditEvents(ctx context.Context, q url.Values) (Page[AuditEvent], error) {
	return List[AuditEvent](ctx, c, "/org/audit-events", q)
}

// EachAuditEvent walks every matching event across pages.
func (c *Client) EachAuditEvent(ctx context.Context, q url.Values, fn func(AuditEvent) error) error {
	return Each(ctx, c, "/audit-events", q, fn)
}

// ExportAuditLog starts a CSV or JSON export. scope is "workspace" or "org"; filter takes action, resourceType, from, to. Poll with WaitExport.
func (c *Client) ExportAuditLog(ctx context.Context, format, scope string, filter map[string]any) (Export, error) {
	body := map[string]any{"format": format, "scope": scope}
	for k, v := range filter {
		body[k] = v
	}
	return Data[Export](ctx, c, Request{Method: http.MethodPost, Path: "/audit-events/export", Body: body})
}

func (c *Client) GetExport(ctx context.Context, id string) (Export, error) {
	return Data[Export](ctx, c, Request{Method: http.MethodGet, Path: "/exports/" + id})
}

// WaitExport polls until the export is done or failed, every interval (default 1.5s) for at most timeout (default 2 minutes).
func (c *Client) WaitExport(ctx context.Context, id string, interval, timeout time.Duration) (Export, error) {
	if interval <= 0 {
		interval = 1500 * time.Millisecond
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	var waited time.Duration
	for {
		e, err := c.GetExport(ctx, id)
		if err != nil || e.Export.Status == "done" || e.Export.Status == "failed" {
			return e, err
		}
		if waited >= timeout {
			return e, errors.New("formflow: export did not finish in time")
		}
		if err := c.o.Sleep(ctx, interval); err != nil {
			return e, err
		}
		waited += interval
	}
}

func (c *Client) ListTables(ctx context.Context) ([]Table, error) {
	return Data[[]Table](ctx, c, Request{Method: http.MethodGet, Path: "/tables"})
}
func (c *Client) GetTable(ctx context.Context, id string) (Table, error) {
	return Data[Table](ctx, c, Request{Method: http.MethodGet, Path: "/tables/" + id})
}
func (c *Client) CreateTable(ctx context.Context, name string, columns []map[string]any) (Table, error) {
	return Data[Table](ctx, c, Request{Method: http.MethodPost, Path: "/tables", Body: map[string]any{"name": name, "columns": columns}})
}
func (c *Client) DeleteTable(ctx context.Context, id string) error {
	return c.Do(ctx, Request{Method: http.MethodDelete, Path: "/tables/" + id}, nil)
}
func (c *Client) ListTableRows(ctx context.Context, id string, q url.Values) (Page[map[string]any], error) {
	return List[map[string]any](ctx, c, "/tables/"+id+"/rows", q)
}
func (c *Client) AddTableRows(ctx context.Context, id string, rows []map[string]any) ([]map[string]any, error) {
	return Data[[]map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/tables/" + id + "/rows", Body: rows})
}

func (c *Client) ListWorkflows(ctx context.Context, formID string) ([]map[string]any, error) {
	q := url.Values{}
	if formID != "" {
		q.Set("formId", formID)
	}
	return Data[[]map[string]any](ctx, c, Request{Method: http.MethodGet, Path: "/workflows", Query: q})
}
func (c *Client) GetWorkflow(ctx context.Context, id string) (map[string]any, error) {
	return Data[map[string]any](ctx, c, Request{Method: http.MethodGet, Path: "/workflows/" + id})
}
func (c *Client) CreateWorkflow(ctx context.Context, body map[string]any) (map[string]any, error) {
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/workflows", Body: body})
}
func (c *Client) UpdateWorkflow(ctx context.Context, id string, patch map[string]any) (map[string]any, error) {
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPatch, Path: "/workflows/" + id, Body: patch})
}
func (c *Client) DeleteWorkflow(ctx context.Context, id string) error {
	return c.Do(ctx, Request{Method: http.MethodDelete, Path: "/workflows/" + id}, nil)
}
func (c *Client) ListWorkflowExecutions(ctx context.Context, id string) ([]map[string]any, error) {
	return Data[[]map[string]any](ctx, c, Request{Method: http.MethodGet, Path: "/workflows/" + id + "/executions"})
}

func (c *Client) ListThemes(ctx context.Context) ([]map[string]any, error) {
	return Data[[]map[string]any](ctx, c, Request{Method: http.MethodGet, Path: "/themes"})
}
func (c *Client) GetTheme(ctx context.Context, id string) (map[string]any, error) {
	return Data[map[string]any](ctx, c, Request{Method: http.MethodGet, Path: "/themes/" + id})
}
func (c *Client) CreateTheme(ctx context.Context, name string, definition map[string]any) (map[string]any, error) {
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/themes", Body: map[string]any{"name": name, "definition": definition}})
}
func (c *Client) UpdateTheme(ctx context.Context, id string, patch map[string]any) (map[string]any, error) {
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPatch, Path: "/themes/" + id, Body: patch})
}
func (c *Client) DeleteTheme(ctx context.Context, id string) error {
	return c.Do(ctx, Request{Method: http.MethodDelete, Path: "/themes/" + id}, nil)
}

// GenerateForm turns a prompt into a validated form definition. opts: presentation, audience, create ({"name": ..., "folderId": ...}). Uses AI credits.
func (c *Client) GenerateForm(ctx context.Context, prompt string, opts map[string]any) (map[string]any, error) {
	body := map[string]any{"prompt": prompt}
	for k, v := range opts {
		body[k] = v
	}
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/ai/generate-form", Body: body})
}

// ListFiles lists the workspace media library. q: search, kind (image|video|font), cursor.
func (c *Client) ListFiles(ctx context.Context, q url.Values) (Page[map[string]any], error) {
	return List[map[string]any](ctx, c, "/media", q)
}

// UploadFile stores bytes in the media library (about 1.3 MB; larger files use multipart POST /api/media).
func (c *Client) UploadFile(ctx context.Context, filename string, content []byte, meta map[string]any) (map[string]any, error) {
	body := map[string]any{"filename": filename, "contentBase64": base64.StdEncoding.EncodeToString(content)}
	for k, v := range meta {
		body[k] = v
	}
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/media", Body: body})
}

// UploadFileFromURL has the server fetch a public URL into the media library.
func (c *Client) UploadFileFromURL(ctx context.Context, filename, fromURL string) (map[string]any, error) {
	return Data[map[string]any](ctx, c, Request{Method: http.MethodPost, Path: "/media", Body: map[string]any{"filename": filename, "fromUrl": fromURL}})
}
