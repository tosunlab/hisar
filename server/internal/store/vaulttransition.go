package store

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tryhisar/hisar/core/apitypes"
)

var ErrTransitionLimit = errors.New("store: another vault transition is still being staged")

type vaultTransition struct {
	apitypes.VaultTransition
	vaultID   string
	actorID   string
	expiresAt time.Time
	encName   []byte
}

const transitionColumns = `id::text, vault_id::text, actor_id, kind, state, generation, item_seq, access_revision, metadata_revision,
 source_name, coalesce(target_id::text,''), coalesce(target_generation,0), coalesce(target_seq,0),
 coalesce(target_access_revision,0), coalesce(target_metadata_revision,0), item_count, expires_at, enc_name`

func readTransition(ctx context.Context, tx queryer, vaultID, actorID, id string, lock bool) (*vaultTransition, error) {
	query := `SELECT ` + transitionColumns + ` FROM vault_transitions WHERE id=$1 AND vault_id=$2 AND actor_id=$3`
	if lock {
		query += ` FOR UPDATE`
	}
	op := &vaultTransition{}
	target := &apitypes.VaultSnapshot{}
	err := tx.QueryRow(ctx, query, id, vaultID, actorID).Scan(&op.ID, &op.vaultID, &op.actorID, &op.Kind, &op.State,
		&op.Source.KeyGeneration, &op.Source.ItemSeq, &op.Source.AccessRevision, &op.Source.MetadataRevision,
		&op.SourceName, &op.TargetID, &target.KeyGeneration, &target.ItemSeq, &target.AccessRevision, &target.MetadataRevision,
		&op.ItemCount, &op.expiresAt, &op.encName)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	op.Protocol = apitypes.VaultTransitionProtocol
	if op.TargetID != "" {
		op.Target = target
	}
	return op, nil
}

func snapshotOf(vault *lockedVault) apitypes.VaultSnapshot {
	return apitypes.VaultSnapshot{KeyGeneration: vault.keyGeneration, ItemSeq: vault.seq,
		AccessRevision: vault.accessRevision, MetadataRevision: vault.metadataRevision}
}

// lockTransition keeps the organization-before-vault order used by all access changes.
func lockTransition(ctx context.Context, tx pgx.Tx, op *vaultTransition) (map[string]*lockedVault, error) {
	if err := lockVaultOrganization(ctx, tx, op.vaultID); err != nil {
		return nil, err
	}
	ids := []string{op.vaultID}
	if op.TargetID != "" {
		ids = append(ids, op.TargetID)
	}
	vaults, err := lockVaults(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	if len(vaults) != len(ids) {
		return nil, ErrNotFound
	}
	if err := requireVaultOwner(ctx, tx, op.vaultID, op.actorID); err != nil {
		return nil, err
	}
	if snapshotOf(vaults[op.vaultID]) != op.Source {
		return nil, ErrConflict
	}
	if op.Target != nil {
		if snapshotOf(vaults[op.TargetID]) != *op.Target {
			return nil, ErrConflict
		}
		if err := requireVaultWriter(ctx, tx, op.TargetID, op.actorID); err != nil {
			return nil, err
		}
		if err := vaults[op.TargetID].checkItemKeyGeneration(op.Target.KeyGeneration); err != nil {
			return nil, err
		}
	}
	return vaults, nil
}

// CleanupVaultTransitions removes a bounded amount of abandoned staging, never active user records.
func (s *Store) CleanupVaultTransitions(ctx context.Context) error {
	return s.runInTransaction(ctx, func(tx pgx.Tx) error {
		for _, table := range []string{"vault_transition_items", "vault_transition_access"} {
			_, err := tx.Exec(ctx, `WITH abandoned AS (
			 SELECT id FROM vault_transitions t WHERE (state <> 'staging' OR expires_at <= now())
			 AND EXISTS(SELECT 1 FROM `+table+` r WHERE r.transition_id=t.id)
			 ORDER BY id LIMIT 10 FOR UPDATE SKIP LOCKED)
			 DELETE FROM `+table+` WHERE ctid IN (SELECT r.ctid FROM `+table+` r
			 JOIN abandoned t ON t.id=r.transition_id LIMIT 1000)`)
			if err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `DELETE FROM vault_transitions t WHERE id IN (SELECT id FROM vault_transitions t
		 WHERE expires_at <= now() AND NOT EXISTS (SELECT 1 FROM vault_transition_items WHERE transition_id=t.id)
		 AND NOT EXISTS (SELECT 1 FROM vault_transition_access WHERE transition_id=t.id) LIMIT 100 FOR UPDATE SKIP LOCKED)`)
		return err
	})
}

func (s *Store) BeginVaultTransition(ctx context.Context, vaultID, actorID string, request *apitypes.BeginVaultTransitionRequest) (*apitypes.VaultTransition, error) {
	if err := s.CleanupVaultTransitions(ctx); err != nil {
		return nil, err
	}
	var result *apitypes.VaultTransition
	err := s.runInTransaction(ctx, func(tx pgx.Tx) error {
		if existing, err := readTransition(ctx, tx, vaultID, actorID, request.ID, false); err == nil {
			if existing.Kind != request.Kind || existing.Source.KeyGeneration != request.KeyGeneration || existing.TargetID != request.TargetID ||
				(existing.Target != nil && existing.Target.KeyGeneration != request.TargetKeyGeneration) {
				return ErrConflict
			}
			result = &existing.VaultTransition
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := lockVaultOrganization(ctx, tx, vaultID); err != nil {
			return err
		}
		ids := []string{vaultID}
		if request.TargetID != "" {
			ids = append(ids, request.TargetID)
		}
		vaults, err := lockVaults(ctx, tx, ids)
		if err != nil {
			return err
		}
		if len(vaults) != len(ids) {
			return ErrNotFound
		}
		if err := requireVaultOwner(ctx, tx, vaultID, actorID); err != nil {
			return err
		}
		source := vaults[vaultID]
		if source.keyGeneration != request.KeyGeneration {
			return ErrStaleKeyGeneration
		}
		// A new operation supersedes the actor's own earlier ones on this vault. A crashed or suspended app cannot cancel
		// an operation whose ID it lost, and only one replacement of a generation can ever commit.
		if _, err := tx.Exec(ctx, `DELETE FROM vault_transitions WHERE vault_id=$1 AND actor_id=$2 AND state='aborted'`, vaultID, actorID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `UPDATE vault_transitions SET state='aborted' WHERE vault_id=$1 AND actor_id=$2 AND state='staging' RETURNING id::text`, vaultID, actorID)
		if err != nil {
			return err
		}
		superseded, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if err := releaseStaging(ctx, tx, superseded); err != nil {
			return err
		}
		var pending int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM vault_transitions t WHERE vault_id=$1 AND
		 ((state='staging' AND expires_at>now()) OR EXISTS(SELECT 1 FROM vault_transition_items WHERE transition_id=t.id)
		  OR EXISTS(SELECT 1 FROM vault_transition_access WHERE transition_id=t.id))`, vaultID).Scan(&pending); err != nil {
			return err
		}
		if pending >= 2 {
			return ErrTransitionLimit
		}
		var name []byte
		if err := tx.QueryRow(ctx, `SELECT enc_name FROM vaults WHERE id=$1`, vaultID).Scan(&name); err != nil {
			return err
		}
		var target *lockedVault
		if request.Kind == "delete-move" {
			if source.vaultType == "personal" {
				var isPrimary bool
				if err := tx.QueryRow(ctx, `SELECT $1::uuid = (SELECT id FROM vaults WHERE created_by = $2 AND type = 'personal' ORDER BY created_at, id LIMIT 1)`, vaultID, actorID).Scan(&isPrimary); err != nil {
					return err
				}
				if isPrimary {
					return ErrPersonalVault
				}
			}
			if err := source.checkItemKeyGeneration(request.KeyGeneration); err != nil {
				return err
			}
			var vaultCount int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM vault_access WHERE account_id=$1`, actorID).Scan(&vaultCount); err != nil {
				return err
			}
			if vaultCount <= 1 {
				return ErrOnlyVault
			}
			target = vaults[request.TargetID]
			if err := requireVaultWriter(ctx, tx, request.TargetID, actorID); err != nil {
				return err
			}
			if err := target.checkItemKeyGeneration(request.TargetKeyGeneration); err != nil {
				return err
			}
		}
		var targetGen, targetSeq, targetAccess, targetMeta any
		if target != nil {
			targetGen, targetSeq, targetAccess, targetMeta = target.keyGeneration, target.seq, target.accessRevision, target.metadataRevision
		}
		var byteBudget int64
		if err := tx.QueryRow(ctx, `SELECT coalesce(sum(octet_length(overview)::bigint+octet_length(details)+256),0)
		 + CASE WHEN $2='rotate' THEN (SELECT count(*) FROM vault_access WHERE vault_id=$1)*$3 ELSE 0 END
		 FROM items WHERE vault_id=$1 AND ($2='rotate' OR NOT deleted)`, vaultID, request.Kind, apitypes.MaxVaultWrappedKeyBytes).Scan(&byteBudget); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO vault_transitions(id,vault_id,actor_id,kind,generation,item_seq,access_revision,metadata_revision,source_name,
		 target_id,target_generation,target_seq,target_access_revision,target_metadata_revision,byte_budget)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::uuid,$11,$12,$13,$14,$15)`,
			request.ID, vaultID, actorID, request.Kind, source.keyGeneration, source.seq, source.accessRevision, source.metadataRevision, name,
			request.TargetID, targetGen, targetSeq, targetAccess, targetMeta, byteBudget)
		if isUniqueViolation(err) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		op, err := readTransition(ctx, tx, vaultID, actorID, request.ID, false)
		if err == nil {
			result = &op.VaultTransition
		}
		return err
	})
	return result, err
}

func (s *Store) GetVaultTransition(ctx context.Context, vaultID, actorID, id string) (*apitypes.VaultTransition, error) {
	op, err := readTransition(ctx, s.pool, vaultID, actorID, id, false)
	if err != nil {
		return nil, err
	}
	if op.State == "staging" && !op.expiresAt.After(time.Now()) {
		op.State = "aborted"
	}
	return &op.VaultTransition, nil
}

func (s *Store) withStagingTransition(ctx context.Context, vaultID, actorID, id string, fn func(pgx.Tx, *vaultTransition) error) error {
	return s.runInTransaction(ctx, func(tx pgx.Tx) error {
		op, err := readTransition(ctx, tx, vaultID, actorID, id, false)
		if err != nil {
			return err
		}
		if _, err := lockTransition(ctx, tx, op); err != nil {
			return err
		}
		op, err = readTransition(ctx, tx, vaultID, actorID, id, true)
		if err != nil {
			return err
		}
		if op.State != "staging" || !op.expiresAt.After(time.Now()) {
			return ErrConflict
		}
		if err := fn(tx, op); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE vault_transitions SET expires_at=now()+interval '1 hour' WHERE id=$1`, id)
		return err
	})
}

func (s *Store) StageVaultItems(ctx context.Context, vaultID, actorID, id string, items []apitypes.RotatedItem) error {
	return s.withStagingTransition(ctx, vaultID, actorID, id, func(tx pgx.Tx, op *vaultTransition) error {
		var delta int64
		for _, item := range items {
			var matches bool
			var previousBytes int64
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM items WHERE vault_id=$1 AND id=$2 AND revision=$3 AND deleted=$4
			 AND ($5='rotate' OR NOT deleted)),coalesce((SELECT octet_length(overview)+octet_length(details)
			 FROM vault_transition_items WHERE transition_id=$6 AND id=$2),0)`, vaultID, item.ID, item.BaseRevision, item.Deleted, op.Kind, id).Scan(&matches, &previousBytes); err != nil {
				return err
			}
			if !matches {
				return ErrConflict
			}
			tag, err := tx.Exec(ctx, `INSERT INTO vault_transition_items(transition_id,id,base_revision,revision,deleted,overview,details)
			 VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(transition_id,id) DO UPDATE SET id=excluded.id
			 WHERE (vault_transition_items.base_revision,vault_transition_items.revision,vault_transition_items.deleted,vault_transition_items.overview,vault_transition_items.details)
			 IS NOT DISTINCT FROM (excluded.base_revision,excluded.revision,excluded.deleted,excluded.overview,excluded.details)`,
				id, item.ID, item.BaseRevision, item.Revision, item.Deleted, item.Overview, item.Details)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrConflict
			}
			delta += int64(len(item.Overview)+len(item.Details)) - previousBytes
		}
		return chargeTransitionBytes(ctx, tx, id, delta)
	})
}

func (s *Store) StageVaultAccess(ctx context.Context, vaultID, actorID, id string, accesses []apitypes.RotatedAccess) error {
	return s.withStagingTransition(ctx, vaultID, actorID, id, func(tx pgx.Tx, op *vaultTransition) error {
		if op.Kind != "rotate" {
			return ErrConflict
		}
		var delta int64
		for _, access := range accesses {
			var role string
			var previousBytes int64
			err := tx.QueryRow(ctx, `SELECT coalesce((SELECT role FROM vault_access WHERE vault_id=$1 AND account_id=$2),''),
			 coalesce((SELECT octet_length(wrapped_key) FROM vault_transition_access WHERE transition_id=$3 AND account_id=$2),0)`, vaultID, access.AccountID, id).Scan(&role, &previousBytes)
			if err != nil {
				return err
			}
			if role != access.Role {
				return ErrRotationMembers
			}
			tag, err := tx.Exec(ctx, `INSERT INTO vault_transition_access(transition_id,account_id,role,wrapped_key) VALUES($1,$2,$3,$4)
			 ON CONFLICT(transition_id,account_id) DO UPDATE SET account_id=excluded.account_id
			 WHERE (vault_transition_access.role,vault_transition_access.wrapped_key) IS NOT DISTINCT FROM (excluded.role,excluded.wrapped_key)`, id, access.AccountID, access.Role, access.WrappedKey)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrConflict
			}
			delta += int64(len(access.WrappedKey)) - previousBytes
		}
		return chargeTransitionBytes(ctx, tx, id, delta)
	})
}

func chargeTransitionBytes(ctx context.Context, tx pgx.Tx, id string, delta int64) error {
	if delta == 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE vault_transitions SET staged_bytes=staged_bytes+$2 WHERE id=$1 AND staged_bytes+$2<=byte_budget`, id, delta)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrResourceBudget
	}
	return nil
}

// ListVaultTransitionItems returns a revision-bound keyset page, including a largest legal record within the byte bound.
func (s *Store) ListVaultTransitionItems(ctx context.Context, vaultID, actorID, id, after string) ([]Item, string, error) {
	items := []Item{}
	var next string
	err := s.withStagingTransition(ctx, vaultID, actorID, id, func(tx pgx.Tx, op *vaultTransition) error {
		rows, err := tx.Query(ctx, `SELECT id,revision,seq,deleted,CASE WHEN fits THEN overview END,CASE WHEN fits THEN details END,updated_at,updated_by,fits
		 FROM (SELECT *, sum(octet_length(overview)+octet_length(details)) OVER(ORDER BY id)-octet_length(overview)-octet_length(details)<4194304 AS fits
		 FROM items WHERE vault_id=$1 AND ($2='' OR id>NULLIF($2,'')::uuid) AND ($3='rotate' OR NOT deleted) ORDER BY id LIMIT 100) p ORDER BY id`, vaultID, after, op.Kind)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Item
			var fits bool
			if err := rows.Scan(&item.ID, &item.Revision, &item.Seq, &item.Deleted, &item.Overview, &item.Details, &item.UpdatedAt, &item.UpdatedBy, &fits); err != nil {
				return err
			}
			if !fits {
				break
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(items) > 0 {
			last := items[len(items)-1].ID
			rows.Close()
			var more bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM items WHERE vault_id=$1 AND id>$2::uuid AND ($3='rotate' OR NOT deleted))`, vaultID, last, op.Kind).Scan(&more); err != nil {
				return err
			}
			if more {
				next = last
			}
		}
		return nil
	})
	return items, next, err
}

func (s *Store) AbortVaultTransition(ctx context.Context, vaultID, actorID, id string) error {
	err := s.runInTransaction(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE vault_transitions SET state='aborted' WHERE id=$1 AND vault_id=$2 AND actor_id=$3 AND state='staging'`, id, vaultID, actorID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			_, err := readTransition(ctx, tx, vaultID, actorID, id, false)
			return err
		}
		return releaseStaging(ctx, tx, []string{id})
	})
	if err != nil {
		return err
	}
	return s.CleanupVaultTransitions(ctx)
}

// releaseStaging deletes the staged copies of operations that can no longer commit, in the transaction that ends them,
// so they never hold another operation's admission.
func releaseStaging(ctx context.Context, tx pgx.Tx, ids []string) error {
	for _, table := range []string{"vault_transition_items", "vault_transition_access"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE transition_id = ANY($1::uuid[])`, ids); err != nil {
			return err
		}
	}
	return nil
}

// CommitVaultTransition checks exact coverage and publishes only a complete replacement under the original authority.
func (s *Store) CommitVaultTransition(ctx context.Context, vaultID, actorID, id string, encName []byte) (*apitypes.VaultTransition, error) {
	var result *apitypes.VaultTransition
	err := s.runInTransaction(ctx, func(tx pgx.Tx) error {
		op, err := readTransition(ctx, tx, vaultID, actorID, id, false)
		if err != nil {
			return err
		}
		if op.State == "committed" {
			if !bytes.Equal(op.encName, encName) {
				return ErrConflict
			}
			result = &op.VaultTransition
			return nil
		}
		vaults, err := lockTransition(ctx, tx, op)
		if err != nil {
			if current, lookupErr := readTransition(ctx, tx, vaultID, actorID, id, false); lookupErr == nil && current.State == "committed" && bytes.Equal(current.encName, encName) {
				result = &current.VaultTransition
				return nil
			}
			return err
		}
		op, err = readTransition(ctx, tx, vaultID, actorID, id, true)
		if err != nil {
			return err
		}
		if op.State != "staging" || !op.expiresAt.After(time.Now()) {
			return ErrConflict
		}
		var mismatch bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM
		 (SELECT id,revision,deleted FROM items WHERE vault_id=$1 AND ($3='rotate' OR NOT deleted)) live
		 FULL JOIN (SELECT id,base_revision,deleted FROM vault_transition_items WHERE transition_id=$2) staged USING(id)
		 WHERE live.id IS NULL OR staged.id IS NULL OR live.revision<>staged.base_revision OR live.deleted<>staged.deleted)`, vaultID, id, op.Kind).Scan(&mismatch)
		if err != nil {
			return err
		}
		if mismatch {
			return ErrConflict
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM vault_transition_items WHERE transition_id=$1`, id).Scan(&op.ItemCount); err != nil {
			return err
		}
		if op.Kind == "rotate" {
			if len(encName) == 0 {
				return ErrConflict
			}
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM (SELECT account_id,role FROM vault_access WHERE vault_id=$1) live
			 FULL JOIN (SELECT account_id,role FROM vault_transition_access WHERE transition_id=$2) staged USING(account_id)
			 WHERE live.account_id IS NULL OR staged.account_id IS NULL OR live.role<>staged.role)`, vaultID, id).Scan(&mismatch)
			if err != nil {
				return err
			}
			if mismatch {
				return ErrRotationMembers
			}
			before, err := HistoryUsage(ctx, tx, vaultID)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM item_history WHERE vault_id=$1`, vaultID); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `WITH staged AS (SELECT *,row_number() OVER(ORDER BY id) AS position FROM vault_transition_items WHERE transition_id=$2)
			 UPDATE items i SET revision=s.revision,seq=$3+s.position,deleted=s.deleted,overview=s.overview,details=s.details,updated_at=now(),updated_by=$4
			 FROM staged s WHERE i.vault_id=$1 AND i.id=s.id`, vaultID, id, op.Source.ItemSeq, actorID)
			if err != nil {
				return err
			}
			if err := insertTransitionHistory(ctx, tx, vaultID, id); err != nil {
				return err
			}
			if err := CheckHistoryGrowth(ctx, tx, vaultID, before, false); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE vault_access a SET wrapped_key=s.wrapped_key,role=s.role,granted_by=$3,granted_at=now()
			 FROM vault_transition_access s WHERE s.transition_id=$2 AND a.vault_id=$1 AND a.account_id=s.account_id`, vaultID, id, actorID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE vaults SET enc_name=$2,key_generation=key_generation+1,needs_rotation=false,seq=$3 WHERE id=$1`, vaultID, encName, op.Source.ItemSeq+op.ItemCount); err != nil {
				return err
			}
		} else {
			if len(encName) != 0 {
				return ErrConflict
			}
			if vaults[vaultID].needsRotation {
				return ErrRotationPending
			}
			before, err := HistoryUsage(ctx, tx, op.TargetID)
			if err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM items i JOIN vault_transition_items s ON s.id=i.id
			 WHERE s.transition_id=$1 AND i.vault_id=$2 AND NOT i.deleted)`, id, op.TargetID).Scan(&mismatch); err != nil {
				return err
			}
			if mismatch {
				return ErrExists
			}
			_, err = tx.Exec(ctx, `INSERT INTO items(vault_id,id,revision,seq,deleted,overview,details,updated_by)
			 SELECT $1,id,revision,$3+row_number() OVER(ORDER BY id),false,overview,details,$4 FROM vault_transition_items WHERE transition_id=$2
			 ON CONFLICT(vault_id,id) DO UPDATE SET revision=excluded.revision,seq=excluded.seq,deleted=false,overview=excluded.overview,details=excluded.details,updated_at=now(),updated_by=excluded.updated_by`, op.TargetID, id, op.Target.ItemSeq, actorID)
			if err != nil {
				return err
			}
			if err := insertTransitionHistory(ctx, tx, op.TargetID, id); err != nil {
				return err
			}
			if err := CheckHistoryGrowth(ctx, tx, op.TargetID, before, false); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE vaults SET seq=$2 WHERE id=$1`, op.TargetID, op.Target.ItemSeq+op.ItemCount); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM vaults WHERE id=$1`, vaultID); err != nil {
				return err
			}
		}
		if err := releaseStaging(ctx, tx, []string{id}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE vault_transitions SET state='committed',enc_name=$2,item_count=$3,expires_at=clock_timestamp()+interval '1 hour' WHERE id=$1`, id, encName, op.ItemCount); err != nil {
			return err
		}
		op.State = "committed"
		result = &op.VaultTransition
		return nil
	})
	return result, err
}

func insertTransitionHistory(ctx context.Context, tx pgx.Tx, vaultID, id string) error {
	_, err := tx.Exec(ctx, `INSERT INTO item_history(vault_id,id,revision,deleted,overview,details,updated_at,updated_by)
	 SELECT i.vault_id,i.id,i.revision,i.deleted,i.overview,i.details,i.updated_at,i.updated_by FROM items i
	 JOIN vault_transition_items s ON i.id=s.id WHERE i.vault_id=$1 AND s.transition_id=$2`, vaultID, id)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}
