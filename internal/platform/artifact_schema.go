package platform

import "context"

func (s *Service) MigrateArtifacts(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS suma_release_workers(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,token_hash VARCHAR(64) NOT NULL UNIQUE,version INTEGER NOT NULL,status VARCHAR(20) NOT NULL,profiles TEXT NOT NULL,deployment INTEGER NOT NULL,created_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,worker_id))`,
		`CREATE TABLE IF NOT EXISTS suma_artifact_uploads(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,token_hash VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,profile_id VARCHAR(100) NOT NULL,profile_sha256 VARCHAR(64) NOT NULL,manifest_sha256 VARCHAR(64) NOT NULL,manifest TEXT NOT NULL,size_bytes BIGINT NOT NULL,state VARCHAR(20) NOT NULL,artifact_id VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,run_id,node_id,attempt,fence))`,
		`CREATE TABLE IF NOT EXISTS suma_artifact_blobs(upload_id VARCHAR(64) NOT NULL,digest VARCHAR(64) NOT NULL,size_bytes BIGINT NOT NULL,storage_path VARCHAR(200) NOT NULL,PRIMARY KEY(upload_id,digest))`,
		`CREATE TABLE IF NOT EXISTS suma_release_artifacts(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,upload_id VARCHAR(64) NOT NULL UNIQUE,manifest_sha256 VARCHAR(64) NOT NULL,manifest TEXT NOT NULL,size_bytes BIGINT NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_artifact_bindings(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,source_node_id VARCHAR(150) NOT NULL,binding TEXT NOT NULL,PRIMARY KEY(run_id,node_id,attempt))`,
		`CREATE TABLE IF NOT EXISTS suma_artifact_proofs(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,kind VARCHAR(30) NOT NULL,artifact_id VARCHAR(64) NOT NULL,manifest_sha256 VARCHAR(64) NOT NULL,source_commit VARCHAR(64) NOT NULL,profile_id VARCHAR(100) NOT NULL,profile_sha256 VARCHAR(64) NOT NULL,workspace_sha256 VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,UNIQUE(run_id,node_id,attempt,kind))`,
		`CREATE TABLE IF NOT EXISTS suma_release_candidates(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,name VARCHAR(200) NOT NULL,payload TEXT NOT NULL,created_at VARCHAR(40) NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(q)); err != nil {
			return err
		}
	}
	return nil
}
