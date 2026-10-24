package cli

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWebhookDeliversExactSignedBodyWithoutSessionCredentials(t *testing.T) {
	secret := strings.Repeat("12", 32)
	t.Setenv("SUMA_WEBHOOK_SECRET", secret)
	t.Setenv("SUMA_TOKEN", "must-not-be-forwarded")
	entry, delivery := strings.Repeat("a", 32), "ci-delivery-20261024"
	body := "{\n  \"schema_version\":1,\"event\":\"ci.completed\",\"external_id\":\"ci-42\",\"conclusion\":\"success\"\n}\n"
	called := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		got, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/api/v1/hooks/"+entry || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || string(got) != body {
			t.Error("signed delivery altered raw body, route, or forwarded session credentials")
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Suma-Delivery") != delivery || r.Header.Get("X-Suma-Timestamp") == "" {
			t.Error("missing protocol headers")
		}
		mac := hmac.New(sha256.New, []byte(secret))
		fmt.Fprintf(mac, "suma-v1\n%s\n%s\n%s\n%s", entry, r.Header.Get("X-Suma-Timestamp"), delivery, body)
		if r.Header.Get("X-Suma-Signature-256") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
			t.Error("signature does not bind exact bytes, entry, timestamp and delivery")
		}
		w.WriteHeader(202)
		io.WriteString(w, `{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","state":"accepted","duplicate":false,"occurrence_id":"cccccccccccccccccccccccccccccccc"}`)
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--control", server.URL, "webhook", "deliver", "--ingress", entry, "--delivery", delivery, "--file", "-"}, strings.NewReader(body), &out, &stderr)
	if code != 0 || called != 1 || !strings.Contains(out.String(), `"accepted"`) || stderr.Len() != 0 || strings.Contains(out.String(), secret) {
		t.Fatalf("code=%d calls=%d out=%s err=%s", code, called, out.String(), stderr.String())
	}
}

func TestWebhookRejectsUnsafeFlagsBodyAndSecretBeforeNetwork(t *testing.T) {
	secret := strings.Repeat("ab", 32)
	t.Setenv("SUMA_WEBHOOK_SECRET", secret)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	base := []string{"--control", server.URL, "webhook", "deliver", "--ingress", strings.Repeat("a", 32), "--delivery", "delivery-1", "--file", "-"}
	for _, body := range []string{"[]", "null", "{} {}", "{\"x\":\"" + strings.Repeat("a", webhookBodyLimit) + "\"}", string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})} {
		var out, stderr bytes.Buffer
		if code := Execute(context.Background(), base, strings.NewReader(body), &out, &stderr); code != 2 || out.Len() != 0 {
			t.Fatalf("bad body code=%d", code)
		}
	}
	for _, tail := range [][]string{{"--secret-env", "missing-env"}, {"--delivery", "bad\r\nheader"}, {"--secret", "forbidden-key-argument"}, {"--ingress", "../other"}} {
		var out, stderr bytes.Buffer
		if code := Execute(context.Background(), append(append([]string{}, base...), tail...), strings.NewReader(`{}`), &out, &stderr); code != 2 || strings.Contains(stderr.String(), secret) {
			t.Fatalf("bad arguments code=%d", code)
		}
	}
	t.Setenv("SUMA_WEBHOOK_SECRET", "invalid-signing-secret-must-not-echo")
	var out, stderr bytes.Buffer
	if code := Execute(context.Background(), base, strings.NewReader(`{}`), &out, &stderr); code != 2 || strings.Contains(stderr.String(), "invalid-signing-secret-must-not-echo") {
		t.Fatal("invalid signing secret escaped")
	}
	if calls != 0 {
		t.Fatalf("unsafe input reached network %d times", calls)
	}
}

func TestWebhookRejectsRemotePlainHTTPAndDoesNotEchoResponse(t *testing.T) {
	secret := strings.Repeat("cd", 32)
	t.Setenv("SUMA_WEBHOOK_SECRET", secret)
	args := []string{"webhook", "deliver", "--ingress", strings.Repeat("a", 32), "--delivery", "delivery-1", "--file", "-"}
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), append([]string{"--control", "http://suma.invalid"}, args...), strings.NewReader(`{}`), &out, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "HTTPS") {
		t.Fatal("remote unencrypted receiver allowed")
	}
	for _, status := range []int{403, 202} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprintf(w, `{"error":"%s","raw_body":"untrusted-payload-value"}`, secret)
			}))
			defer server.Close()
			out.Reset()
			stderr.Reset()
			code = Execute(context.Background(), append([]string{"--control", server.URL}, args...), strings.NewReader(`{}`), &out, &stderr)
			if code != 1 || out.Len() != 0 || strings.Contains(stderr.String(), secret) || strings.Contains(stderr.String(), "untrusted-payload-value") {
				t.Fatal("untrusted response escaped into CLI output")
			}
		})
	}
}

func TestWebhookRedirectNeverForwardsSignatureAndUnknownOutcomeIsExplicit(t *testing.T) {
	t.Setenv("SUMA_LANGUAGE", "en-US")
	t.Setenv("SUMA_WEBHOOK_SECRET", strings.Repeat("ef", 32))
	forwarded := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded++ }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--control", server.URL, "webhook", "deliver", "--ingress", strings.Repeat("a", 32), "--delivery", "delivery-1", "--file", "-"}, strings.NewReader(`{}`), &out, &stderr)
	if code != 1 || forwarded != 0 || out.Len() != 0 || !strings.Contains(stderr.String(), "outcome may be unknown") || !strings.Contains(stderr.String(), "same ingress, delivery ID and exact body") {
		t.Fatal("redirect or uncertain-result recovery unsafe")
	}
}

func TestWebhookFileRetryKeepsBytesAndRecoversDuplicateReceipt(t *testing.T) {
	t.Setenv("SUMA_WEBHOOK_SECRET", strings.Repeat("23", 32))
	body := []byte("{\"schema_version\":1,\"event\":\"monitor.firing\",\"external_id\":\"alert-42\"}\r\n")
	file := filepath.Join(t.TempDir(), "monitor.json")
	if err := os.WriteFile(file, body, 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if !bytes.Equal(raw, body) || r.Header.Get("X-Suma-Delivery") != "stable-alert-42" {
			t.Error("retry changed identity or raw bytes")
		}
		if calls == 1 {
			w.WriteHeader(202)
		}
		fmt.Fprintf(w, `{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","state":"accepted","duplicate":%t,"occurrence_id":"cccccccccccccccccccccccccccccccc"}`, calls > 1)
	}))
	defer server.Close()
	for i := 0; i < 2; i++ {
		var out, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"--control", server.URL, "webhook", "deliver", "--ingress", strings.Repeat("a", 32), "--delivery", "stable-alert-42", "--file", file}, strings.NewReader(""), &out, &stderr)
		if code != 0 || stderr.Len() != 0 || !strings.Contains(out.String(), fmt.Sprintf(`"duplicate": %t`, i > 0)) {
			t.Fatalf("retry %d code=%d", i, code)
		}
	}
}

func TestWebhookTimeoutLeavesDeliveryIdentityForRecovery(t *testing.T) {
	t.Setenv("SUMA_LANGUAGE", "en-US")
	t.Setenv("SUMA_WEBHOOK_SECRET", strings.Repeat("45", 32))
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
		close(finished)
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--control", server.URL, "--timeout", "50ms", "webhook", "deliver", "--ingress", strings.Repeat("a", 32), "--delivery", "stable-timeout-42", "--file", "-"}, strings.NewReader(`{}`), &out, &stderr)
	if code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "outcome may be unknown") {
		t.Fatal("timeout presented as definitive no-effect")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("request context not cancelled")
	}
}
