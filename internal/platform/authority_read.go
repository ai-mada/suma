package platform

import "context"

// withCurrentRead checks current credential, source, scope and membership in
// the exact transaction that reads the response. The callback uses auth.tx;
// opening a second DB/Engine connection here can deadlock SQLite's single pool.
// File/index/network work runs after this bounded transaction, with a fresh
// current proof before delivering a new response batch.
func (s *Service) withCurrentRead(ctx context.Context, actor Actor, target commandTarget, purpose commandPurpose, read func(*authorizedCommand) error) error {
	if read == nil {
		return invalid("读取操作不能为空")
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = func() error {
			bounded, tx, cleanup, e := s.beginAuthorityTx(ctx)
			if e != nil {
				return e
			}
			defer cleanup()
			defer tx.Rollback()
			auth, e := s.authorizeTx(bounded, tx, actor, target, purpose)
			if e != nil {
				return e
			}
			if e = read(auth); e != nil {
				return e
			}
			if e = auth.assertTx(tx, target, purpose); e != nil {
				return e
			}
			return tx.Commit()
		}()
		if !authoritySetRetry(err) {
			return err
		}
	}
	return err
}
