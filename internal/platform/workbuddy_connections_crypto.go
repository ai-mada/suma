package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/workbuddy"
)

// This private wire exists only inside authenticated encryption. TokenGrant's
// safe JSON representation deliberately excludes token fields.
type workBuddySecret struct {
	Access         string `json:"access"`
	Refresh        string `json:"refresh"`
	RefreshPresent bool   `json:"refresh_present"`
}

func (workBuddySecret) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workBuddySecret{credentials:redacted}")
}
func (w *WorkBuddyConnections) workBuddyAEAD() (cipher.AEAD, error) {
	if w == nil || len(w.key) != 32 {
		return nil, fmt.Errorf("%s", workBuddyUnavailable)
	}
	block, err := aes.NewCipher(w.key)
	if err != nil {
		return nil, fmt.Errorf("%s", workBuddyUnavailable)
	}
	return cipher.NewGCM(block)
}
func workBuddyAAD(row workBuddyConnectionRow) ([]byte, error) {
	scopes, err := workBuddyCanonicalScopes(row.Scopes, false)
	if err != nil {
		return nil, err
	}
	if row.TenantID == "" || row.ProjectID == "" || row.ID == "" || row.OwnerID == "" || row.ClientID == "" || row.OpenID == "" || row.GrantGeneration < 1 || row.TokenGeneration < 1 {
		return nil, fmt.Errorf("%s", workBuddyInvalid)
	}
	return json.Marshal([]any{"workbuddy-grant-v1", row.TenantID, row.ProjectID, row.ID, row.OwnerID, row.ClientID, row.OpenID, row.AppDigest, row.GrantGeneration, row.TokenGeneration, scopes, row.ExpiresAt})
}
func (w *WorkBuddyConnections) seal(row workBuddyConnectionRow, grant workbuddy.TokenGrant) (string, error) {
	aead, err := w.workBuddyAEAD()
	if err != nil {
		return "", err
	}
	aad, err := workBuddyAAD(row)
	if err != nil {
		return "", err
	}
	expires, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil || !expires.Equal(grant.ExpiresAt) || row.OpenID != grant.OpenID {
		return "", fmt.Errorf("%s", workBuddyInvalid)
	}
	scopes, err := workBuddyCanonicalScopes(grant.Scopes, false)
	if err != nil {
		return "", err
	}
	rowScopes, _ := workBuddyCanonicalScopes(row.Scopes, false)
	if stringify(scopes) != stringify(rowScopes) || grant.AccessToken == "" || len(grant.AccessToken) > 8192 || len(grant.RefreshToken) > 8192 || grant.RefreshTokenPresent != (grant.RefreshToken != "") {
		return "", fmt.Errorf("%s", workBuddyInvalid)
	}
	raw, err := json.Marshal(workBuddySecret{grant.AccessToken, grant.RefreshToken, grant.RefreshTokenPresent})
	if err != nil {
		return "", fmt.Errorf("%s", workBuddyInvalid)
	}
	defer func() {
		for i := range raw {
			raw[i] = 0
		}
	}()
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("%s", workBuddyUnavailable)
	}
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, raw, aad)), nil
}
func (w *WorkBuddyConnections) open(row workBuddyConnectionRow) (workbuddy.TokenGrant, error) {
	fail := func() (workbuddy.TokenGrant, error) {
		return workbuddy.TokenGrant{}, fmt.Errorf("%s", workBuddyUnavailable)
	}
	aead, err := w.workBuddyAEAD()
	if err != nil {
		return fail()
	}
	aad, err := workBuddyAAD(row)
	if err != nil {
		return fail()
	}
	if len(row.Ciphertext) > 32768 {
		return fail()
	}
	raw, err := base64.StdEncoding.DecodeString(row.Ciphertext)
	if err != nil || len(raw) < aead.NonceSize()+aead.Overhead() {
		return fail()
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], aad)
	if err != nil {
		return fail()
	}
	defer func() {
		for i := range plain {
			plain[i] = 0
		}
	}()
	var secret workBuddySecret
	decoder := json.NewDecoder(strings.NewReader(string(plain)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&secret) != nil || secret.Access == "" || len(secret.Access) > 8192 || len(secret.Refresh) > 8192 || secret.RefreshPresent != (secret.Refresh != "") {
		return fail()
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fail()
	}
	expires, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil {
		return fail()
	}
	scopes, _ := workBuddyCanonicalScopes(row.Scopes, false)
	return workbuddy.TokenGrant{AccessToken: secret.Access, RefreshToken: secret.Refresh, RefreshTokenPresent: secret.RefreshPresent, OpenID: row.OpenID, Scopes: scopes, ExpiresAt: expires}, nil
}
func (w *WorkBuddyConnections) stateToken(op workBuddyOAuthOperation) string {
	if w == nil || len(w.key) != 32 || op.ID == "" {
		return ""
	}
	var scopes []string
	if json.Unmarshal([]byte(op.ScopesJSON), &scopes) != nil {
		return ""
	}
	scopes, err := workBuddyCanonicalScopes(scopes, false)
	if err != nil {
		return ""
	}
	body := stringify([]any{"workbuddy-oauth-state-v1", op.ID, op.ConnectionID, op.TenantID, op.ProjectID, op.OwnerID, op.CredentialDigest, op.AppDigest, scopes, op.ExpiresAt})
	mac := hmac.New(sha256.New, w.key)
	_, _ = mac.Write([]byte(body))
	return op.ID + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (w *WorkBuddyConnections) verifyState(op workBuddyOAuthOperation, state string) bool {
	expected := w.stateToken(op)
	if expected == "" || state == "" || len(state) > 256 || len(op.StateHash) != 64 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(state)) == 1 && subtle.ConstantTimeCompare([]byte(digest(state)), []byte(op.StateHash)) == 1
}
