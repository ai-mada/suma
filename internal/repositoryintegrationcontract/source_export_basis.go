package repositoryintegrationcontract

import (
	"fmt"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

// RepositorySourceExportBasis describes the current observations of one
// configured delivery branch and its designated baseline. This metadata is
// separate from the immutable request and Git bundle that it helps construct.
type RepositorySourceExportBasis struct {
	Source   RepositoryTargetObservation `json:"source"`
	Baseline RepositoryTargetObservation `json:"baseline"`
}

func ValidateRepositorySourceExportBasis(alias string, basis RepositorySourceExportBasis) error {
	if basis.Source.Alias != alias || basis.Baseline.Alias == alias || basis.Source.Ref == basis.Baseline.Ref || basis.Source.ObjectFormat != basis.Baseline.ObjectFormat {
		return fmt.Errorf("repository export pair changed")
	}
	for _, obs := range []RepositoryTargetObservation{basis.Source, basis.Baseline} {
		want := 40
		if obs.ObjectFormat == "sha256" {
			want = 64
		}
		if !obs.Available || obs.Reason != "" || !sourceIdentity.MatchString(obs.Alias) || !RepositoryValidRef(obs.Ref) || !cc.ValidHash(obs.ConfigSHA256) ||
			(obs.ObjectFormat != "sha1" && obs.ObjectFormat != "sha256") || !cc.ValidCommit(obs.HeadCommit) || len(obs.HeadCommit) != want ||
			obs.FenceOID != "" && (!cc.ValidCommit(obs.FenceOID) || len(obs.FenceOID) != want) ||
			obs.Generation < 0 || obs.Generation >= 9007199254740990 || (obs.Generation == 0) != (obs.FenceOID == "") {
			return fmt.Errorf("invalid repository export observation")
		}
		if _, err := time.Parse(time.RFC3339Nano, obs.ObservedAt); err != nil {
			return fmt.Errorf("invalid repository export observation time")
		}
	}
	return nil
}
