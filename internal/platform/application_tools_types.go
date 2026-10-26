package platform

import "encoding/json"

// ApplicationToolDefinition is the shared application contract. MCP annotations
// describe effects; only the current domain authorization grants permission.
// Parameters contains a self-contained JSON Schema, never remote schema URLs.
type ApplicationToolDefinition struct {
	Name             string         `json:"name"`
	MCPName          string         `json:"mcp_name"`
	Description      string         `json:"description"`
	DescriptionKey   string         `json:"description_key"`
	Parameters       map[string]any `json:"parameters"`
	ReadOnly         bool           `json:"read_only"`
	CredentialScopes []string       `json:"credential_scopes"`
}

type ApplicationToolList struct {
	Items []ApplicationToolDefinition `json:"items"`
}

// The project is selected by the authenticated transport. Tool arguments may
// never override it or supply an Actor, endpoint, credential or organization.
// Write tools carry their stable idempotency key inside their typed arguments.
type ApplicationToolCall struct {
	ProjectID string          `json:"project_id"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

// DeveloperConnectionMetadata declares server capabilities. It never asserts
// that a client is connected or that a newly issued credential was exercised.
type DeveloperConnectionMetadata struct {
	ProjectID                string                      `json:"project_id"`
	EndpointPath             string                      `json:"endpoint_path"`
	EndpointURL              string                      `json:"endpoint_url,omitempty"`
	EndpointSource           string                      `json:"endpoint_source"`
	Transport                string                      `json:"transport"`
	Stateless                bool                        `json:"stateless"`
	SDKName                  string                      `json:"sdk_name"`
	SDKVersion               string                      `json:"sdk_version"`
	TokenEnvironment         string                      `json:"token_environment"`
	ExpectedUserID           string                      `json:"expected_user_id"`
	ExpectedOrganizationID   string                      `json:"expected_organization_id"`
	ClientConnectionVerified bool                        `json:"client_connection_verified"`
	Tools                    []ApplicationToolDefinition `json:"tools"`
}
