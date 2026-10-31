package runner

import "testing"

// These vectors also drive desktop-opencode-connector-model.test.ts. The Web
// comparison protects recovery of the original fixed configuration: escaping
// differences must never mark a changed endpoint or credential reference ready.
func TestOpenCodeConfigurationDigestMatchesWebRecoveryVectors(t *testing.T) {
	for _, vector := range []struct{ name, executable, expected string }{
		{"ascii_html", "C:/tools/OpenCode <>&/opencode.exe", "c35514a91c9b68a3810da809b403ffad8244b2cb76589c464258b846fd195cb2"},
		{"unicode_html", "C:/工具/<>&/opencode.exe", "42afdd7217c7e12d8e9b8eebab9ffb91506679a8790a783d0496c41210661035"},
	} {
		t.Run(vector.name, func(t *testing.T) {
			config := OpenCodeSpec{Executable: vector.executable, ProviderID: "provider-one", Model: "vendor/model-one", BaseURL: "https://provider.example/v1", APIKeyEnvRef: "PROVIDER_API_KEY"}
			if got := OpenCodeConfigurationDigest(config); got != vector.expected {
				t.Fatalf("Go/Web fixed-configuration digest mismatch: got %s, want %s", got, vector.expected)
			}
		})
	}
}
