package formflow

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// SignPayload builds a FormFlow-Signature header value: t=<unix>,v1=<hex HMAC-SHA256(secret, "t.body")>.
func SignPayload(secret string, body []byte, t int64) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(strconv.FormatInt(t, 10) + "."))
	m.Write(body)
	return "t=" + strconv.FormatInt(t, 10) + ",v1=" + hex.EncodeToString(m.Sum(nil))
}

// VerifySignature reports whether header carries a matching v1 signature inside the tolerance window
// (default 5 minutes when tolerance is 0). Pass the RAW request body. Several v1 values are accepted for secret rotation.
func VerifySignature(secret string, body []byte, header string, tolerance time.Duration, now time.Time) bool {
	if header == "" {
		return false
	}
	if tolerance == 0 {
		tolerance = 5 * time.Minute
	}
	var t int64
	var haveT bool
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return false
			}
			t, haveT = n, true
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if !haveT {
		return false
	}
	if d := now.Sub(time.Unix(t, 0)); d > tolerance || d < -tolerance {
		return false
	}
	want := strings.SplitN(SignPayload(secret, body, t), "v1=", 2)[1]
	for _, s := range sigs {
		if hmac.Equal([]byte(s), []byte(want)) {
			return true
		}
	}
	return false
}

// Event is a verified webhook delivery. Data stays raw so callers decode only the types they handle.
type Event struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Created     int64           `json:"created"`
	WorkspaceID string          `json:"workspace_id"`
	Data        json.RawMessage `json:"data"`
}

// ConstructEvent verifies the signature and parses the event. It returns *Error{Code: "invalid_signature"} on failure.
func ConstructEvent(secret string, body []byte, header string) (Event, error) {
	var e Event
	if !VerifySignature(secret, body, header, 0, time.Now()) {
		return e, &Error{Status: 400, Code: "invalid_signature", Message: "Webhook signature verification failed"}
	}
	if err := json.Unmarshal(body, &e); err != nil {
		return e, &Error{Status: 400, Code: "invalid_payload", Message: "Webhook body is not JSON"}
	}
	return e, nil
}
