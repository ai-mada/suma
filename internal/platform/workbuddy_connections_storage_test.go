package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
	"github.com/ai-mada/suma/internal/workbuddy"
)

func workBuddyStorageFixture(t *testing.T) (*WorkBuddyConnections, workBuddyConnectionRow, workbuddy.TokenGrant) {
	t.Helper()
	w, err := NewWorkBuddyConnections(&Service{}, workbuddy.OAuthConfig{ClientID: "app-one", ClientSecret: "fixture-secret", RedirectURI: "https://suma.example/callback", Scopes: []string{"user.task.readable"}}, bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	r := workBuddyConnectionRow{WorkBuddyConnection: WorkBuddyConnection{ID: "connection-one", ProjectID: "project-one", OwnerID: "owner-one", ClientID: "app-one", OpenID: "open-one", Version: 1, Scopes: []string{"user.task.readable"}, State: "connected", GrantGeneration: 1, TokenGeneration: 1, ExpiresAt: "2030-01-01T00:00:00Z"}, TenantID: "tenant-one", AppDigest: w.appDigest}
	g := workbuddy.TokenGrant{AccessToken: "private-access", RefreshToken: "private-refresh", RefreshTokenPresent: true, OpenID: r.OpenID, Scopes: r.Scopes, ExpiresAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	return w, r, g
}

func TestWorkBuddyConnectionsStorageCipherIsolation(t *testing.T) {
	w, r, g := workBuddyStorageFixture(t)
	ciphertext, err := w.seal(r, g)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, g.AccessToken) || strings.Contains(ciphertext, g.RefreshToken) {
		t.Fatal("plaintext credential persisted")
	}
	r.Ciphertext = ciphertext
	got, err := w.open(r)
	if err != nil || got.AccessToken != g.AccessToken || got.RefreshToken != g.RefreshToken {
		t.Fatal("grant failed round trip", err)
	}
	other, err := w.seal(r, g)
	if err != nil || other == ciphertext {
		t.Fatal("encryption nonce reused", err)
	}
	for _, change := range []func(*workBuddyConnectionRow){func(r *workBuddyConnectionRow) { r.TenantID = "other" }, func(r *workBuddyConnectionRow) { r.ProjectID = "other" }, func(r *workBuddyConnectionRow) { r.ID = "other" }, func(r *workBuddyConnectionRow) { r.OwnerID = "other" }, func(r *workBuddyConnectionRow) { r.ClientID = "other" }, func(r *workBuddyConnectionRow) { r.OpenID = "other" }, func(r *workBuddyConnectionRow) { r.GrantGeneration++ }, func(r *workBuddyConnectionRow) { r.TokenGeneration++ }, func(r *workBuddyConnectionRow) { r.Scopes = []string{"user.task.invokable"} }} {
		changed := r
		change(&changed)
		if _, err = w.open(changed); err == nil {
			t.Fatal("credential accepted with changed audience")
		}
	}
	wrong := *w
	wrong.key = bytes.Repeat([]byte{8}, 32)
	if _, err = wrong.open(r); err == nil {
		t.Fatal("wrong key accepted")
	}
}

func TestWorkBuddyConnectionsStorageStateAndConfigurationIsolation(t *testing.T) {
	w, r, _ := workBuddyStorageFixture(t)
	op := workBuddyOAuthOperation{ID: "operation-one", TenantID: r.TenantID, ProjectID: r.ProjectID, OwnerID: r.OwnerID, ConnectionID: r.ID, CredentialDigest: strings.Repeat("a", 64), AppDigest: w.appDigest, ScopesJSON: `["user.task.readable"]`, ExpiresAt: "2030-01-01T00:00:00Z"}
	state := w.stateToken(op)
	op.StateHash = digest(state)
	if state == "" || !w.verifyState(op, state) {
		t.Fatal("bound state rejected")
	}
	for _, change := range []func(*workBuddyOAuthOperation){func(o *workBuddyOAuthOperation) { o.CredentialDigest = "other" }, func(o *workBuddyOAuthOperation) { o.TenantID = "other" }, func(o *workBuddyOAuthOperation) { o.ProjectID = "other" }, func(o *workBuddyOAuthOperation) { o.OwnerID = "other" }, func(o *workBuddyOAuthOperation) { o.AppDigest = "other" }, func(o *workBuddyOAuthOperation) { o.ExpiresAt = "2031-01-01T00:00:00Z" }, func(o *workBuddyOAuthOperation) { o.ScopesJSON = `["user.task.invokable"]` }} {
		changed := op
		change(&changed)
		if w.verifyState(changed, state) {
			t.Fatal("state crossed binding")
		}
	}
	if w.verifyState(op, state+"x") {
		t.Fatal("tampered state accepted")
	}
	if _, err := w.oauth([]string{"user.task.invokable"}); err == nil {
		t.Fatal("request scope expanded application scope")
	}
	disabled, err := NewWorkBuddyConnections(&Service{}, workbuddy.OAuthConfig{}, nil)
	if err != nil || disabled.configured {
		t.Fatal("empty configuration not disabled", err)
	}
	if got := disabled.connectionView(r, true).AllowedActions; len(got) != 1 || got[0] != "disconnect" {
		t.Fatal("disabled instance must preserve only local disconnect")
	}
	for _, state := range []string{"authorization_unknown", "refresh_unknown", "authorizing", "refreshing"} {
		r.State = state
		for _, action := range w.connectionView(r, true).AllowedActions {
			if action == "refresh" {
				t.Fatal("uncertain operation offered refresh")
			}
		}
	}
}

func TestWorkBuddyConnectionsStorageConfigurationChangesKeepLocalRecovery(t *testing.T) {
	w, row, grant := workBuddyStorageFixture(t)
	var err error
	row.Ciphertext, err = w.seal(row, grant)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.connectionView(row, true).AllowedActions; strings.Join(got, ",") != "authorize,refresh,disconnect" {
		t.Fatal("valid connection lost actions", got)
	}
	narrowed := *w
	narrowed.app = w.app
	narrowed.app.Scopes = []string{"user.task.invokable"}
	if got := narrowed.connectionView(row, true).AllowedActions; strings.Join(got, ",") != "authorize,disconnect" {
		t.Fatal("removed scope still refreshable", got)
	}
	if _, err = narrowed.oauth(row.Scopes); err == nil {
		t.Fatal("removed scope admitted network client")
	}
	for _, change := range []func(*WorkBuddyConnections){func(w *WorkBuddyConnections) { w.configured = false }, func(w *WorkBuddyConnections) { w.appDigest = digest("changed-application") }, func(w *WorkBuddyConnections) { w.key = nil }} {
		unavailable := *w
		change(&unavailable)
		if got := unavailable.connectionView(row, true).AllowedActions; strings.Join(got, ",") != "disconnect" {
			t.Fatal("configuration outage blocked local disconnect or offered provider action", got)
		}
		if len(unavailable.connectionView(row, false).AllowedActions) != 0 {
			t.Fatal("read permission became disconnect authority")
		}
		ended := row
		ended.State = "disconnected"
		if len(unavailable.connectionView(ended, true).AllowedActions) != 0 {
			t.Fatal("disconnected unavailable connection offered action")
		}
	}
}

func TestWorkBuddyConnectionsStorageKeyLifecycle(t *testing.T) {
	dir := t.TempDir()
	empty := func(string) string { return "" }
	k, err := LoadWorkBuddyConnectionKey(dir, "sqlite", empty)
	if err != nil || len(k) != 32 {
		t.Fatal("key initialization", err)
	}
	again, err := LoadWorkBuddyConnectionKey(dir, "sqlite", empty)
	if err != nil || !bytes.Equal(k, again) {
		t.Fatal("key changed on restart", err)
	}
	if _, err = LoadWorkBuddyConnectionKey(dir, "postgres", empty); err == nil {
		t.Fatal("shared database generated independent key")
	}
	for _, encoded := range []string{"bad", base64.StdEncoding.EncodeToString(make([]byte, 31))} {
		if _, err = LoadWorkBuddyConnectionKey(dir, "sqlite", func(string) string { return encoded }); err == nil {
			t.Fatal("invalid configured key accepted")
		}
	}
	broken := t.TempDir()
	path := filepath.Join(broken, "workbuddy-connections.key")
	if err = os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadWorkBuddyConnectionKey(broken, "sqlite", empty); err == nil {
		t.Fatal("partial key repaired")
	}
	original, _ := os.ReadFile(path)
	if string(original) != "original" {
		t.Fatal("existing key overwritten")
	}
}

func TestWorkBuddyConnectionsStorageSchemaRejectsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name       string
		registered bool
		ddls       []string
	}{
		{"unregistered", false, []string{`CREATE TABLE suma_workbuddy_connections(original TEXT)`}},
		{"partial", true, []string{`CREATE TABLE suma_workbuddy_connections(original TEXT)`}},
		{"wrong_columns", true, []string{`CREATE TABLE suma_workbuddy_connections(id TEXT PRIMARY KEY)`, `CREATE TABLE suma_workbuddy_oauth_operations(id TEXT PRIMARY KEY)`}},
		{"wrong_pk", true, []string{strings.Replace(workBuddyConnectionsDDL, "id VARCHAR(64) PRIMARY KEY", "id VARCHAR(64) NOT NULL", 1), workBuddyOAuthOperationsDDL}},
		{"extra_column", true, []string{strings.TrimSuffix(workBuddyConnectionsDDL, ")") + ", legacy TEXT)", workBuddyOAuthOperationsDDL}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "storage.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ctx := context.Background()
			if tc.registered {
				if err = store.InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", store.InitialDataContract); err != nil {
					t.Fatal(err)
				}
			}
			for _, ddl := range tc.ddls {
				if _, err = db.Exec(ddl); err != nil {
					t.Fatal(err)
				}
			}
			var before, after int
			db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&before)
			s := &Service{DB: db, Dialect: "sqlite"}
			if err = s.InitializeWorkBuddyConnections(ctx); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatal("unknown schema accepted", err)
			}
			db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&after)
			if after != before {
				t.Fatal("rejection changed schema")
			}
		})
	}
	ctx := context.Background()
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "valid.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Service{DB: db, Dialect: "sqlite"}
	if err = s.InitializeWorkBuddyConnections(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.InitializeWorkBuddyConnections(ctx); err != nil {
		t.Fatal("valid restart rejected", err)
	}
}

func TestWorkBuddyConnectionsStorageCurrentAuthorityAndCAS(t *testing.T) {
	s, p, token, project := authorityFixture(t)
	ctx := context.Background()
	if err := s.InitializeWorkBuddyConnections(ctx); err != nil {
		t.Fatal(err)
	}
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	w, row, grant := workBuddyStorageFixture(t)
	w.s = s
	row.TenantID = p.TenantID
	row.ProjectID = project
	row.OwnerID = p.ID
	row.Ciphertext, err = w.seal(row, grant)
	if err != nil {
		t.Fatal(err)
	}
	write := func(run func(*authorizedCommand) error) error {
		bounded, tx, close, e := s.beginAuthorityTx(ctx)
		if e != nil {
			return e
		}
		defer close()
		defer tx.Rollback()
		auth, e := s.authorizeTx(bounded, tx, actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: row.ID}, purposeBrowserProjectWrite)
		if e != nil {
			return e
		}
		if e = run(auth); e != nil {
			return e
		}
		if e = auth.assertTx(auth.tx, auth.target, purposeBrowserProjectWrite); e != nil {
			return e
		}
		return auth.tx.Commit()
	}
	if err = write(func(a *authorizedCommand) error { return w.saveRowTx(ctx, a, row, 0) }); err != nil {
		t.Fatal(err)
	}
	row.Version = 2
	if err = write(func(a *authorizedCommand) error { return w.saveRowTx(ctx, a, row, 1) }); err != nil {
		t.Fatal(err)
	}
	if err = write(func(a *authorizedCommand) error { return w.saveRowTx(ctx, a, row, 1) }); err == nil {
		t.Fatal("stale connection CAS accepted")
	}
	other := row
	other.Version = 3
	other.OwnerID = "another-person"
	if err = write(func(a *authorizedCommand) error { return w.saveRowTx(ctx, a, other, 2) }); err == nil {
		t.Fatal("owner authority bypassed")
	}
	op := workBuddyOAuthOperation{ID: newID(), TenantID: p.TenantID, ProjectID: project, ConnectionID: row.ID, OwnerID: p.ID, Kind: "refresh", State: "exchanging", RequestHash: digest("request"), CredentialDigest: actor.credential.digest, AppDigest: w.appDigest, ScopesJSON: `["user.task.readable"]`, Version: 1, ConnectionVersion: 2, GrantGeneration: 1, TokenGeneration: 1}
	if err = write(func(a *authorizedCommand) error { return w.saveOpTx(ctx, a, op, 0) }); err != nil {
		t.Fatal(err)
	}
	op.Version = 2
	op.State = "unknown"
	if err = write(func(a *authorizedCommand) error { return w.saveOpTx(ctx, a, op, 1) }); err != nil {
		t.Fatal(err)
	}
	if err = write(func(a *authorizedCommand) error { return w.saveOpTx(ctx, a, op, 1) }); err == nil {
		t.Fatal("stale operation CAS accepted")
	}
	if err = write(func(a *authorizedCommand) error {
		got, e := w.rowTx(ctx, a.tx, p.TenantID, project, row.ID, true)
		if e != nil {
			return e
		}
		if got.Version != 2 || len(got.AllowedActions) != 0 {
			return errors.New("persisted dynamic actions or incorrect row")
		}
		stored, e := w.opTx(ctx, a.tx, p.TenantID, project, op.ID, true)
		if e != nil {
			return e
		}
		if stored.State != "unknown" || stored.Version != 2 {
			return errors.New("operation CAS not preserved")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = logoutCredentialForTest(ctx, s, token); err != nil {
		t.Fatal(err)
	}
	if err = write(func(a *authorizedCommand) error { return w.saveRowTx(ctx, a, row, 1) }); err == nil {
		t.Fatal("revoked source admitted storage")
	}
}

func TestWorkBuddyConnectionsStorageFinalVersionCAS(t *testing.T) {
	s, p, token, project := authorityFixture(t)
	ctx := context.Background()
	if err := s.InitializeWorkBuddyConnections(ctx); err != nil {
		t.Fatal(err)
	}
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	w, row, grant := workBuddyStorageFixture(t)
	w.s = s
	row.TenantID, row.ProjectID, row.OwnerID = p.TenantID, project, p.ID
	row.Ciphertext, err = w.seal(row, grant)
	if err != nil {
		t.Fatal(err)
	}
	op := workBuddyOAuthOperation{ID: newID(), TenantID: p.TenantID, ProjectID: project, ConnectionID: row.ID, OwnerID: p.ID, Kind: "refresh", State: "unknown", RequestHash: digest("request"), CredentialDigest: actor.credential.digest, AppDigest: w.appDigest, ScopesJSON: `["user.task.readable"]`, Version: 1, ConnectionVersion: 1, GrantGeneration: 1, TokenGeneration: 1}
	bounded, tx, close, err := s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	defer tx.Rollback()
	auth, err := s.authorizeTx(bounded, tx, actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: row.ID}, purposeBrowserProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.saveRowTx(bounded, auth, row, 0); err != nil {
		t.Fatal(err)
	}
	if err = w.saveOpTx(bounded, auth, op, 0); err != nil {
		t.Fatal(err)
	}
	// Seed only this disposable fixture near the boundary; use real CAS for the last write.
	for _, seed := range []struct{ table, id string }{{"suma_workbuddy_connections", row.ID}, {"suma_workbuddy_oauth_operations", op.ID}} {
		if _, err = tx.ExecContext(bounded, s.q("UPDATE "+seed.table+" SET version=? WHERE id=?"), math.MaxInt32-1, seed.id); err != nil {
			t.Fatal(err)
		}
	}
	row.Version, op.Version = math.MaxInt32, math.MaxInt32
	rowErr := w.saveRowTx(bounded, auth, row, math.MaxInt32-1)
	opErr := w.saveOpTx(bounded, auth, op, math.MaxInt32-1)
	if rowErr != nil || opErr != nil {
		t.Fatalf("last legal CAS rejected: connection=%v operation=%v", rowErr, opErr)
	}
	storedRow, err := w.rowTx(bounded, tx, p.TenantID, project, row.ID, true)
	if err != nil || storedRow.Version != math.MaxInt32 {
		t.Fatal("last connection version unreadable", err)
	}
	storedOp, err := w.opTx(bounded, tx, p.TenantID, project, op.ID, true)
	if err != nil || storedOp.Version != math.MaxInt32 {
		t.Fatal("last operation version unreadable", err)
	}
	if err = auth.assertTx(tx, auth.target, purposeBrowserProjectWrite); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkBuddyConnectionsStorageActionsRespectVersionRoom(t *testing.T) {
	w, row, grant := workBuddyStorageFixture(t)
	var err error
	row.Ciphertext, err = w.seal(row, grant)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		version int
		want    string
	}{{math.MaxInt32, ""}, {math.MaxInt32 - 1, "disconnect"}, {math.MaxInt32 - 2, "refresh,disconnect"}, {math.MaxInt32 - 3, "authorize,refresh,disconnect"}} {
		row.Version = tc.version
		if got := strings.Join(w.connectionView(row, true).AllowedActions, ","); got != tc.want {
			t.Errorf("version %d: offered %q, want %q", tc.version, got, tc.want)
		}
	}
	row.Version = math.MaxInt32 - 2
	for _, exhausted := range []string{"token", "grant"} {
		changed := row
		if exhausted == "token" {
			changed.TokenGeneration = math.MaxInt32
		} else {
			changed.GrantGeneration = math.MaxInt32
		}
		if got := strings.Join(w.connectionView(changed, true).AllowedActions, ","); got != "disconnect" {
			t.Errorf("exhausted %s generation: offered %q", exhausted, got)
		}
	}
	row.Version = math.MaxInt32 - 3
	for _, state := range []string{"connected", "authorization_unknown"} {
		changed := row
		changed.State = state
		changed.GrantGeneration = math.MaxInt32 - 1
		want := "disconnect"
		if state == "connected" {
			want = "refresh,disconnect"
		}
		if got := strings.Join(w.connectionView(changed, true).AllowedActions, ","); got != want {
			t.Errorf("%s with one grant transition remaining: offered %q, want %q", state, got, want)
		}
		changed.GrantGeneration = 1
		changed.TokenGeneration = math.MaxInt32
		if got := strings.Join(w.connectionView(changed, true).AllowedActions, ","); got != "disconnect" {
			t.Errorf("%s with exhausted token generation: offered %q", state, got)
		}
	}
}
