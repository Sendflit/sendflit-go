package sendflit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyWebhookSignature(t *testing.T) {
	body := []byte(`{"type":"email.sent"}`)
	mac := hmac.New(sha256.New, []byte("whsec_t"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !VerifyWebhookSignature("whsec_t", body, sig) {
		t.Fatal("valid signature rejected")
	}
	if VerifyWebhookSignature("whsec_t", append(body, ' '), sig) {
		t.Fatal("tampered body accepted")
	}
	if VerifyWebhookSignature("whsec_t", body, "sha256=deadbeef") {
		t.Fatal("bad signature accepted")
	}
	if VerifyWebhookSignature("whsec_t", body, "nope") {
		t.Fatal("unprefixed signature accepted")
	}
}

func TestBatchValidation(t *testing.T) {
	c := New("re_x")
	if _, err := c.Batch(context.Background(), nil); err == nil {
		t.Fatal("empty batch accepted")
	}
	big := make([]Email, 101)
	if _, err := c.Batch(context.Background(), big); err == nil {
		t.Fatal("oversized batch accepted")
	}
}

func TestSendE2E(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"abc","status":"sent"}`))
	}))
	defer srv.Close()

	c := NewWithBaseURL("re_test", srv.URL)
	em := Email{From: "a@b.com", To: "c@d.com", Subject: "Hi", HTML: "<p>x</p>"}
	em.Attach("f.txt", "hi", "")
	res, err := c.Send(context.Background(), em)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.Status != "sent" {
		t.Fatalf("status = %q", res.Status)
	}
	if gotPath != "/v1/email" || gotAuth != "Bearer re_test" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	atts := gotBody["attachments"].([]any)
	att := atts[0].(map[string]any)
	if att["content_b64"] != "aGk=" {
		t.Fatalf("b64 = %v", att["content_b64"])
	}
}

func TestRetryOn500(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n <= 2 {
			w.WriteHeader(500)
			return
		}
		w.Write([]byte(`{"id":"x","status":"sent"}`))
	}))
	defer srv.Close()
	c := NewWithBaseURL("re_t", srv.URL)
	_, err := c.Send(context.Background(), Email{From: "a@b.com", To: "c@d.com", Subject: "s"})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

func TestErrorParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"detail":"forbidden"}`))
	}))
	defer srv.Close()
	c := NewWithBaseURL("re_t", srv.URL)
	_, err := c.Send(context.Background(), Email{From: "a@b.com", To: "c@d.com", Subject: "s"})
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("err = %v", err)
	}
	if ae, ok := err.(*APIError); !ok || ae.Status != 403 {
		t.Fatalf("want APIError 403, got %#v", err)
	}
}
