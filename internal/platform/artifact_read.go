package platform

import (
	"context"
	"sort"
	"strings"
)

// Private read batches defer immutable blob IO until current-authority locks
// have been released. They carry evidence references, never credential authority.
type artifactReadKey struct{}
type artifactReadRef struct {
	path, hash string
	size       int64
}
type artifactReadBatch struct {
	refs     map[string]artifactReadRef
	failures map[string]error
}

func newArtifactReadBatch() *artifactReadBatch {
	return &artifactReadBatch{refs: map[string]artifactReadRef{}, failures: map[string]error{}}
}
func (b *artifactReadBatch) record(refs []artifactReadRef) error {
	for _, r := range refs {
		b.refs[r.path+"/"+r.hash] = r
	}
	for _, r := range refs {
		if err := b.failures[r.path+"/"+r.hash]; err != nil {
			return err
		}
	}
	return nil
}
func (b *artifactReadBatch) fingerprint() string {
	keys := []string{}
	for key, r := range b.refs {
		keys = append(keys, key+"/"+stringify(r.size))
	}
	sort.Strings(keys)
	return digest(strings.Join(keys, "\n"))
}
func (s *Service) withCurrentCRead(ctx context.Context, actor Actor, target commandTarget, purpose commandPurpose, read func(context.Context, *authorizedCommand) error) error {
	first := newArtifactReadBatch()
	run := func(batch *artifactReadBatch) error {
		return s.withCurrentRead(ctx, actor, target, purpose, func(auth *authorizedCommand) error {
			bounded, cancel := context.WithDeadline(context.WithValue(ctx, artifactReadKey{}, batch), auth.deadline)
			defer cancel()
			return read(bounded, auth)
		})
	}
	if err := run(first); err != nil {
		return err
	}
	if len(first.refs) == 0 {
		return nil
	}
	for key, r := range first.refs {
		if _, err := s.readArtifactBlob(r.path, r.hash, r.size); err != nil {
			first.failures[key] = err
		}
	}
	second := newArtifactReadBatch()
	second.failures = first.failures
	if err := run(second); err != nil {
		return err
	}
	if first.fingerprint() != second.fingerprint() {
		return conflict("制品读取依据已变化，请重试")
	}
	return nil
}
