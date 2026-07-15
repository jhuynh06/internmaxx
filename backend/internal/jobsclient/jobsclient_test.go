package jobsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveAddr(t *testing.T) {
	t.Setenv("IMX_API_ADDR", "")
	t.Setenv("API_ADDR", "")
	cases := map[string]string{
		"http://x:9/":    "http://x:9/",
		"https://x":      "https://x",
		"127.0.0.1:8080": "http://127.0.0.1:8080",
		":8080":          "http://127.0.0.1:8080",
		"example.com":    "http://example.com",
	}
	for in, want := range cases {
		if got := ResolveAddr(in); got != want {
			t.Errorf("ResolveAddr(%q) = %q, want %q", in, got, want)
		}
	}

	// Precedence: env fills in when the flag is empty.
	t.Setenv("API_ADDR", ":7000")
	if got := ResolveAddr(""); got != "http://127.0.0.1:7000" {
		t.Errorf("API_ADDR fallback = %q", got)
	}
	t.Setenv("IMX_API_ADDR", "box:6000")
	if got := ResolveAddr(""); got != "http://box:6000" {
		t.Errorf("IMX_API_ADDR should win over API_ADDR, got %q", got)
	}
	if got := ResolveAddr("http://flag:1"); got != "http://flag:1" {
		t.Errorf("flag should win over env, got %q", got)
	}
}

func TestFormatSeen(t *testing.T) {
	if got := FormatSeen("not-a-time"); got != "not-a-time" {
		t.Errorf("bad input should pass through, got %q", got)
	}
	// A valid RFC3339 renders to the local "2006-01-02 15:04" layout (16 chars).
	if got := FormatSeen("2026-07-14T20:50:20Z"); len(got) != 16 {
		t.Errorf("formatted length = %d (%q), want 16", len(got), got)
	}
}

func TestJobs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("company") {
		case "bad":
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid limit"}`))
		case "empty":
			w.Write([]byte(`{"items":null,"limit":20,"offset":0,"total":0}`))
		default:
			w.Write([]byte(`{"items":[{"key":"k","company":"OpenAI","title":"Intern","url":"u","first_seen":"2026-07-14T20:50:20Z"}],"limit":20,"offset":0,"total":1}`))
		}
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, HTTP: srv.Client()}
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		jr, _, err := c.Jobs(ctx, Query{})
		if err != nil || len(jr.Items) != 1 || jr.Items[0].Company != "OpenAI" {
			t.Fatalf("jr=%+v err=%v", jr, err)
		}
	})
	t.Run("null items normalized", func(t *testing.T) {
		jr, _, err := c.Jobs(ctx, Query{Company: "empty"})
		if err != nil || jr.Items == nil || len(jr.Items) != 0 {
			t.Fatalf("items should be [] not nil: %+v err=%v", jr.Items, err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		_, _, err := c.Jobs(ctx, Query{Company: "bad"})
		apiErr, ok := err.(*APIError)
		if !ok || apiErr.Status != 400 || apiErr.Msg != "invalid limit" {
			t.Fatalf("want *APIError{400,invalid limit}, got %v", err)
		}
		if apiErr.Error() != "server error (400): invalid limit" {
			t.Errorf("Error() = %q", apiErr.Error())
		}
	})
}
