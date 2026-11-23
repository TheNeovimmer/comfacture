package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestInvoiceContext(t *testing.T) {
	s := newTestServer(t)
	postForm(s, "/clients", url.Values{"name": {"ACME"}})
	postForm(s, "/invoices", url.Values{
		"client_id": {"1"}, "issue_date": {"2026-11-23"}, "due_date": {"2026-12-01"},
		"desc": {"Dev"}, "qty": {"2"}, "price": {"12.50"},
	})
	ctx, err := s.invoiceContext()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ACME", "25.00 €", "impayée", "2026-12-01"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("context missing %q: %s", want, ctx)
		}
	}
}

func TestChatWithoutAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	s := newTestServer(t)

	r := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"message":"Combien reste-t-il dû ?"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out["error"], "OPENROUTER_API_KEY") {
		t.Errorf("unexpected error %q", out["error"])
	}
}
