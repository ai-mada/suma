package runner

// DesktopManagedClaudeSDKConfiguration is a private main-process/Runner contract.
// The renderer supplies only a scanned native digest, fixed model and environment
// reference; paths and complete asset digests are constructed by the trusted host.
// CredentialEnvRef names a local value, never the credential value itself.
type DesktopManagedClaudeSDKConfiguration struct {
	NativeCLIExecutable   string `json:"native_cli_executable"`
	NativeCLISHA256       string `json:"native_cli_sha256"`
	ReviewedCLIVersion    string `json:"reviewed_cli_version"`
	NodeExecutable        string `json:"node_executable"`
	NodeSHA256            string `json:"node_sha256"`
	RuntimeManifest       string `json:"runtime_manifest"`
	RuntimeManifestSHA256 string `json:"runtime_manifest_sha256"`
	Model                 string `json:"model"`
	CredentialEnvRef      string `json:"credential_env_ref"`
	ConfigurationSHA256   string `json:"configuration_sha256"`
}
