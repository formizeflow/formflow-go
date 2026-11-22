package formflow

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAuditEventsOrgAndExport(t *testing.T) {
	var paths []string
	var body string
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		if strings.HasSuffix(r.URL.Path, "/export") {
			w.WriteHeader(202)
			w.Write([]byte(`{"data":{"export":{"id":"exp_1","status":"queued"}}}`))
			return
		}
		w.Write([]byte(`{"data":[{"id":"aud_1","actorName":"Ada","action":"form.published"}],"nextCursor":null}`))
	})
	ctx := context.Background()
	p, err := c.ListOrgAuditEvents(ctx, url.Values{"action": {"form.published"}})
	if err != nil || len(p.Data) != 1 || p.Data[0].ActorName == nil || *p.Data[0].ActorName != "Ada" {
		t.Fatalf("%v %v", p, err)
	}
	e, err := c.ExportAuditLog(ctx, "json", "org", map[string]any{"resourceType": "form"})
	if err != nil || e.Export.ID != "exp_1" {
		t.Fatalf("%v %v", e, err)
	}
	if paths[0] != "GET /api/v1/org/audit-events?action=form.published" || paths[1] != "POST /api/v1/audit-events/export" {
		t.Fatalf("%v", paths)
	}
	if !strings.Contains(body, `"scope":"org"`) || !strings.Contains(body, `"resourceType":"form"`) || !strings.Contains(body, `"format":"json"`) {
		t.Fatalf("body %s", body)
	}
}

func TestWaitExportPollsUntilDone(t *testing.T) {
	var n int32
	c, slept := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.Write([]byte(`{"data":{"export":{"id":"exp_1","status":"running"}}}`))
			return
		}
		w.Write([]byte(`{"data":{"export":{"id":"exp_1","status":"done"},"url":"https://x.test/f"}}`))
	})
	e, err := c.WaitExport(context.Background(), "exp_1", 0, 0)
	if err != nil || e.URL != "https://x.test/f" || len(*slept) != 2 {
		t.Fatalf("%v %v %v", e, err, *slept)
	}
}

func TestMembersAndAPIKeys(t *testing.T) {
	var got []string
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, r.Method+" "+r.URL.Path+" "+string(b))
		if r.Method == http.MethodDelete {
			w.WriteHeader(204)
			return
		}
		w.Write([]byte(`{"data":{"id":"key_1","key":"ff_live_x","role":"editor"}}`))
	})
	ctx := context.Background()
	k, err := c.CreateAPIKey(ctx, "ci", []string{"form:read"}, 0)
	if err != nil || k.Key != "ff_live_x" {
		t.Fatalf("%v %v", k, err)
	}
	if _, err := c.SetMemberRole(ctx, "mem_1", "editor"); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveMember(ctx, "mem_1"); err != nil {
		t.Fatal(err)
	}
	if got[0] != `POST /api/v1/api-keys {"name":"ci","scopes":["form:read"]}` || got[1] != `PATCH /api/v1/members/mem_1 {"role":"editor"}` || got[2] != "DELETE /api/v1/members/mem_1 " {
		t.Fatalf("%q", got)
	}
}
