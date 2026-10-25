package platform

import (
	"context"
	"database/sql"
	"errors"
)

func delegationOnlyPurpose(p commandPurpose) bool {
	switch p {
	case purposeAutomationEvaluate, purposeAutomationMaterialize, purposeAutomationAdmit, purposeToolAdmission, purposeIngressDelivery:
		return true
	}
	return false
}

// Constructors below consume actual persisted source rows, never a Principal.
// Candidates are not authorization: the transaction rechecks every source.
func (s *Service) durableAutomationActor(ctx context.Context, q authorityQuery, ref automationRef) (Actor, error) {
	a, err := scanAutomation(q.QueryRowContext(ctx, s.q(`SELECT `+automationColumns+` FROM suma_automations WHERE tenant_id=? AND project_id=? AND id=?`), ref.tenant, ref.project, ref.id))
	if err != nil {
		return Actor{}, err
	}
	if a.ActivatedBy == "" || a.Revision < 1 {
		return Actor{}, forbidden()
	}
	return Actor{kind: actorDurableAutomation, delegation: delegationRef{recordID: a.ID, tenantID: ref.tenant, projectID: ref.project, subjectID: a.ActivatedBy, revision: a.Revision}}, nil
}

func (s *Service) durableToolActor(ctx context.Context, q authorityQuery, tenant, project, id string) (Actor, error) {
	p, err := scanToolProfile(q.QueryRowContext(ctx, s.q(`SELECT `+toolProfileColumns+` FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id))
	if err != nil {
		return Actor{}, err
	}
	if p.EnabledBy == "" || p.Version < 1 || p.State != "enabled" {
		return Actor{}, forbidden()
	}
	return Actor{kind: actorDurableToolActivation, delegation: delegationRef{recordID: p.ID, tenantID: tenant, projectID: project, subjectID: p.EnabledBy, revision: p.Version, sourceDigest: p.Profile.ProfileSHA256}}, nil
}

// Human/Runner/pool entry guards already hold the complete organization set.
// Rebuilding this exact persisted tool source does not acquire a late lock or
// borrow its approver's browser session; it can only validate this fixed profile.
func (s *Service) toolActivationLockedTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile) error {
	actor, err := s.durableToolActor(ctx, tx, tenant, project, p.ID)
	if err != nil {
		return err
	}
	d := actor.delegation
	if d.subjectID != p.EnabledBy || d.revision != p.Version || d.sourceDigest != p.Profile.ProfileSHA256 {
		return forbidden()
	}
	_, err = s.currentProjectMembershipLockedTx(ctx, tx, tenant, d.subjectID, project, true, true)
	return err
}

func (s *Service) durableAutomationOccurrenceActor(ctx context.Context, q authorityQuery, ref automationRef, occurrence string) (Actor, error) {
	actor, err := s.durableAutomationActor(ctx, q, ref)
	if err != nil {
		return Actor{}, err
	}
	var source, ingress, user sql.NullString
	err = q.QueryRowContext(ctx, s.q(`SELECT o.source,i.id,i.activated_by FROM suma_automation_occurrences o LEFT JOIN suma_automation_ingress_deliveries d ON d.occurrence_id=o.id AND d.tenant_id=o.tenant_id AND d.project_id=o.project_id LEFT JOIN suma_automation_ingresses i ON i.id=d.ingress_id AND i.tenant_id=o.tenant_id AND i.project_id=o.project_id WHERE o.tenant_id=? AND o.project_id=? AND o.rule_id=? AND o.id=?`), ref.tenant, ref.project, ref.id, occurrence).Scan(&source, &ingress, &user)
	if err != nil {
		return Actor{}, err
	}
	if source.String == "webhook" {
		actor.delegation.linkedRecordID = ingress.String
		actor.delegation.secondarySubjectID = user.String
	}
	return actor, nil
}

// Call only after verifyIngressRequest has verified the real provider signature
// over this exact delivery/body with the snapshot's encrypted current key.
func signedIngressActor(in automationIngressRecord, parent AutomationRule, delivery, bodyHash string) Actor {
	return Actor{kind: actorSignedIngress, delegation: delegationRef{recordID: in.ID, tenantID: in.tenant, projectID: in.ProjectID, subjectID: in.ActivatedBy, secondarySubjectID: parent.ActivatedBy, revision: in.Revision, generation: in.KeyGeneration, sourceDigest: digest(in.ciphertext), bodyDigest: bodyHash, deliveryID: delivery}}
}

func (s *Service) authorizeDelegationTx(ctx context.Context, tx *sql.Tx, actor Actor, target commandTarget, purpose commandPurpose, organizations []string) (*authorizedCommand, error) {
	d := actor.delegation
	if d.recordID == "" || d.tenantID == "" || d.projectID == "" || d.revision < 1 || target.tenantID != d.tenantID || target.projectID != d.projectID {
		return nil, forbidden()
	}
	switch actor.kind {
	case actorDurableAutomation:
		if purpose != purposeAutomationEvaluate && purpose != purposeAutomationMaterialize && purpose != purposeAutomationAdmit {
			return nil, forbidden()
		}
	case actorDurableToolActivation:
		if purpose != purposeToolAdmission || target.resourceID != d.recordID {
			return nil, forbidden()
		}
	case actorSignedIngress:
		if purpose != purposeIngressDelivery || target.resourceID != d.recordID || d.generation < 1 || d.bodyDigest == "" || d.deliveryID == "" {
			return nil, forbidden()
		}
	default:
		return nil, forbidden()
	}
	deadline := s.txAuthorityBudget(ctx, tx).deadline
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := s.authorityWriter(bounded, tx); err != nil {
		return nil, err
	}
	users := sortedAuthorityIDs([]string{d.subjectID, d.secondarySubjectID, target.subjectID})
	orgs := append([]string{d.tenantID}, organizations...)
	for _, user := range users {
		current, err := s.authorityOrganizations(bounded, tx, user)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, current...)
	}
	if err := s.lockHumanAuthorities(bounded, tx, users, orgs, []string{"activation:" + d.recordID}); err != nil {
		return nil, err
	}
	switch actor.kind {
	case actorDurableAutomation:
		a, err := scanAutomation(tx.QueryRowContext(bounded, s.q(s.currentSQL(`SELECT `+automationColumns+` FROM suma_automations WHERE tenant_id=? AND project_id=? AND id=?`)), d.tenantID, d.projectID, d.recordID))
		if err != nil {
			return nil, err
		}
		if a.State != "enabled" || a.Revision != d.revision || a.ActivatedBy != d.subjectID {
			return nil, forbidden()
		}
		if d.linkedRecordID != "" {
			var currentUser, rule string
			err = tx.QueryRowContext(bounded, s.q(s.currentSQL(`SELECT activated_by,rule_id FROM suma_automation_ingresses WHERE tenant_id=? AND project_id=? AND id=?`)), d.tenantID, d.projectID, d.linkedRecordID).Scan(&currentUser, &rule)
			if err != nil {
				return nil, err
			}
			if rule != d.recordID {
				return nil, forbidden()
			}
			if currentUser != d.secondarySubjectID {
				return nil, &APIError{409, "authority_retry", "权限集合已改变，请重试", nil}
			}
		}
		if purpose == purposeAutomationEvaluate || purpose == purposeAutomationMaterialize {
			if target.resourceID != d.recordID {
				return nil, forbidden()
			}
		} else {
			var rule, state string
			var revision int
			err = tx.QueryRowContext(bounded, s.q(s.currentSQL(`SELECT rule_id,revision,state FROM suma_automation_occurrences WHERE tenant_id=? AND project_id=? AND id=?`)), d.tenantID, d.projectID, target.resourceID).Scan(&rule, &revision, &state)
			if err != nil {
				return nil, err
			}
			if rule != d.recordID || revision != d.revision || state != "queued" {
				return nil, forbidden()
			}
		}
	case actorDurableToolActivation:
		p, err := scanToolProfile(tx.QueryRowContext(bounded, s.q(s.currentSQL(`SELECT `+toolProfileColumns+` FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND id=?`)), d.tenantID, d.projectID, d.recordID))
		if err != nil {
			return nil, err
		}
		if p.State != "enabled" || p.EnabledBy != d.subjectID || p.Version != d.revision || p.Profile.ProfileSHA256 != d.sourceDigest {
			return nil, forbidden()
		}
	case actorSignedIngress:
		in, err := scanIngress(tx.QueryRowContext(bounded, s.q(s.currentSQL(`SELECT `+ingressColumns+` FROM suma_automation_ingresses WHERE tenant_id=? AND project_id=? AND id=?`)), d.tenantID, d.projectID, d.recordID))
		if err != nil {
			return nil, err
		}
		if in.Revision != d.revision || in.KeyGeneration != d.generation || digest(in.ciphertext) != d.sourceDigest || in.ActivatedBy != d.subjectID || !in.KeyConfigured {
			return nil, forbidden()
		}
		var parentUser string
		err = tx.QueryRowContext(bounded, s.q(s.currentSQL(`SELECT activated_by FROM suma_automations WHERE tenant_id=? AND project_id=? AND id=?`)), d.tenantID, d.projectID, in.RuleID).Scan(&parentUser)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if parentUser != d.secondarySubjectID {
			return nil, &APIError{409, "authority_retry", "权限集合已改变，请重试", nil}
		}
	}
	at, err := s.authorityNow(bounded, tx)
	if err != nil {
		return nil, err
	}
	proof := &authorizedCommand{tx: tx, actor: actor, target: target, purpose: purpose, checkedAt: at, deadline: deadline, ctx: s.txAuthorityBudget(ctx, tx).ctx}
	if actor.kind == actorSignedIngress {
		// A current verified signature only authorizes the delivery protocol.
		// Admission separately checks both actual activators and the parent rule;
		// historical duplicates do not become new executions after revocation.
		proof.currentPrincipal = Principal{ID: d.subjectID, TenantID: d.tenantID}
		return proof, nil
	}
	p, err := s.currentProjectMembershipLockedTx(bounded, tx, d.tenantID, d.subjectID, d.projectID, true, true)
	if err != nil {
		if purpose != purposeAutomationEvaluate || !automationAuthorityFailure(err) {
			return nil, err
		}
		proof.activationDenied = true
		proof.currentPrincipal = Principal{ID: d.subjectID, TenantID: d.tenantID}
	} else {
		proof.currentPrincipal = p
	}
	if d.secondarySubjectID != "" {
		if _, e := s.currentProjectMembershipLockedTx(bounded, tx, d.tenantID, d.secondarySubjectID, d.projectID, true, true); e != nil {
			if purpose != purposeAutomationEvaluate || !automationAuthorityFailure(e) {
				return nil, e
			}
			proof.activationDenied = true
		}
	}
	return proof, nil
}
