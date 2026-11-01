package runner

import (
	"errors"

	"github.com/zalando/go-keyring"
)

var ErrRunnerSecretNotFound = errors.New("Runner secret is absent from system credential storage")

// RunnerSecretStore deliberately exposes no enumeration, deletion or plaintext
// fallback. Each identity accesses only its own fixed service/account.
type RunnerSecretStore interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
}

type systemRunnerSecretStore struct{}

func SystemRunnerSecretStore() RunnerSecretStore { return systemRunnerSecretStore{} }

func (systemRunnerSecretStore) Get(service, account string) (string, error) {
	secret, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrRunnerSecretNotFound
	}
	if err != nil {
		return "", errors.New("system credential storage is unavailable")
	}
	return secret, nil
}

func (systemRunnerSecretStore) Set(service, account, secret string) error {
	if err := keyring.Set(service, account, secret); err != nil {
		return errors.New("system credential storage could not confirm the write")
	}
	return nil
}
