package cli

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const webhookBodyLimit = 2 << 20

var webhookID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var webhookDeliveryID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{7,127}$`)
var webhookSecret = regexp.MustCompile(`^[a-f0-9]{64}$`)
var webhookSecretEnv = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

type webhookReceipt struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	Duplicate    bool   `json:"duplicate"`
	OccurrenceID string `json:"occurrence_id"`
}

// webhookCommand does not use the session client request method: a signing key
// authorizes only this ingress, and platform credentials must never accompany it.
func webhookCommand(ctx context.Context, c *client, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 || args[0] != "deliver" {
		return usage("webhook requires deliver; see --help")
	}
	fs := commandFlags("webhook deliver")
	entry := fs.String("ingress", "", "ingress ID")
	delivery := fs.String("delivery", "", "stable delivery ID")
	file := fs.String("file", "", "exact event JSON file, or - for stdin")
	secretEnv := fs.String("secret-env", "SUMA_WEBHOOK_SECRET", "process environment name holding the signing key")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || !webhookID.MatchString(*entry) || !webhookDeliveryID.MatchString(*delivery) || *file == "" || !webhookSecretEnv.MatchString(*secretEnv) {
		return usage("webhook deliver requires --ingress (32 hex), --delivery (8..128 safe characters), --file, and a valid --secret-env name")
	}
	origin, _ := url.Parse(c.base)
	ip := net.ParseIP(origin.Hostname())
	if origin.Scheme != "https" && !strings.EqualFold(origin.Hostname(), "localhost") && (ip == nil || !ip.IsLoopback()) {
		return usage("signed delivery requires HTTPS except for loopback development")
	}
	secret := os.Getenv(*secretEnv)
	if !webhookSecret.MatchString(secret) {
		return usage("signing key environment must contain 64 lowercase hex characters; do not pass keys in arguments")
	}
	reader := in
	if *file != "-" {
		f, err := os.Open(*file)
		if err != nil {
			return usage("unable to open event JSON file")
		}
		defer f.Close()
		reader = f
	}
	raw, err := io.ReadAll(io.LimitReader(reader, webhookBodyLimit+1))
	trimmed := bytes.TrimSpace(raw)
	if err != nil || len(raw) > webhookBodyLimit || !utf8.Valid(raw) || len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(raw) {
		return usage("event must be one UTF-8 JSON object at most 2 MiB; exact bytes are retained for signing")
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("suma-v1\n" + *entry + "\n" + timestamp + "\n" + *delivery + "\n"))
	mac.Write(raw)
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/api/v1/hooks/"+*entry, bytes.NewReader(raw))
	if err != nil {
		return usage("invalid ingress request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", string(c.locale))
	req.Header.Set("X-Suma-Delivery", *delivery)
	req.Header.Set("X-Suma-Timestamp", timestamp)
	req.Header.Set("X-Suma-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := c.http.Do(req)
	if err != nil {
		return &failure{Code: "transport", Message: "signed delivery outcome may be unknown; retry the same ingress, delivery ID and exact body; timestamp and signature will be refreshed"}
	}
	defer resp.Body.Close()
	// Never echo an untrusted response/error body: it may reflect the payload or
	// a signing secret. Only the small, explicitly defined receipt is displayed.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return &failure{Code: "webhook_rejected", Message: "signed delivery rejected; inspect the ingress configuration and delivery history before retrying", Status: resp.StatusCode}
	}
	response, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil || len(response) > 64*1024 {
		return &failure{Code: "response", Message: "delivery receipt unavailable; outcome may be unknown; retry the same delivery ID and exact body"}
	}
	var receipt webhookReceipt
	d := json.NewDecoder(bytes.NewReader(response))
	d.DisallowUnknownFields()
	if err := d.Decode(&receipt); err != nil || d.Decode(new(any)) != io.EOF || !webhookID.MatchString(receipt.ID) || receipt.State != "accepted" && receipt.State != "ignored" && receipt.State != "blocked" || receipt.OccurrenceID != "" && !webhookID.MatchString(receipt.OccurrenceID) || receipt.State == "accepted" && receipt.OccurrenceID == "" {
		return &failure{Code: "response", Message: "invalid delivery receipt; outcome may be unknown; retry the same delivery ID and exact body"}
	}
	return writeJSON(out, receipt, secret)
}
