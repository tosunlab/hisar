package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tryhisar/hisar/core/apitypes"
)

var ErrCollectionChanged = errors.New("store: collection changed")
var ErrCollectionTooLarge = errors.New("store: collection record exceeds page budget")

type collectionCursor struct {
	Revision string    `json:"r"`
	After    string    `json:"a"`
	Scope    string    `json:"s"`
	At       time.Time `json:"t"`
}

const publicKeysJSON = `jsonb_build_object('kem', encode(a.kem_pub,'base64'), 'sign', encode(a.sign_pub,'base64'), 'kemSig', encode(a.kem_sig,'base64'))`
const memberJSON = `jsonb_build_object('accountId',a.id,'email',a.email,'name',a.name,'role',a.role,'publicKeys',` + publicKeysJSON + `)`

// collectionQuery returns only static SQL. Account, device, vault and item IDs remain bound parameters.
// Every branch emits key, field, parent and value. Transactional revisions freeze ordering keys between pages.
func collectionQuery(path string) (string, string, string, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch path {
	case "/members":
		return `SELECT to_char(a.created_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||a.id AS key, '' AS field, '' AS parent, ` + memberJSON + ` AS value FROM accounts a WHERE a.id=$1 OR a.id IN (SELECT other.account_id FROM vault_access mine JOIN vault_access other ON other.vault_id=mine.vault_id WHERE mine.account_id=$1)`, "", "", nil
	case "/devices":
		return `SELECT to_char(created_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||id::text AS key,'' AS field,'' AS parent,jsonb_build_object('id',id,'name',name,'platform',platform,'createdAt',created_at,'lastSeenAt',last_seen_at,'current',id::text=$2) AS value FROM devices WHERE account_id=$1 AND revoked_at IS NULL`, "", "", nil
	case "/vaults":
		return `SELECT to_char(v.created_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||v.id::text AS key,'' AS field,'' AS parent,jsonb_build_object('id',v.id,'type',v.type,'encName',encode(v.enc_name,'base64'),'seq',v.seq,'role',a.role,'wrappedKey',encode(a.wrapped_key,'base64'),'organizationId',coalesce(v.organization_id::text,''),'needsRotation',v.needs_rotation,'keyGeneration',v.key_generation,'accessRevision',coalesce((to_jsonb(v)->>'access_revision')::bigint,0),'isPersonalExtra',v.type='personal' AND v.id<>(SELECT p.id FROM vaults p WHERE p.created_by=v.created_by AND p.type='personal' ORDER BY p.created_at,p.id LIMIT 1)) AS value FROM vaults v JOIN vault_access a ON a.vault_id=v.id WHERE a.account_id=$1`, "", "", nil
	case "/share-links":
		return `SELECT id::text AS key,'' AS field,'' AS parent,jsonb_build_object('id',id,'encryptedLabel',encode(encrypted_label,'base64'),'createdAt',created_at,'expiresAt',expires_at,'maxViews',max_views,'viewCount',view_count,'hasPassphrase',passphrase_salt IS NOT NULL) AS value FROM share_links WHERE account_id=$1 AND expires_at>now()`, "", "", nil
	case "/organizations":
		return `SELECT 'o/'||lower(o.name)||'/'||o.id::text AS key,'organizations' AS field,'' AS parent,jsonb_build_object('id',o.id,'name',o.name,'role',m.role,'memberCount',(SELECT count(*) FROM organization_members c WHERE c.organization_id=o.id)) AS value FROM organization_members m JOIN organizations o ON o.id=m.organization_id WHERE m.account_id=$1
 UNION ALL SELECT 'i/'||to_char(i.created_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||i.id::text,'invitations','',jsonb_build_object('id',i.id,'organizationId',o.id,'organizationName',o.name,'role',i.role,'invitedByName',a.name,'invitedByEmail',a.email,'createdAt',i.created_at,'expiresAt',i.expires_at) FROM organization_invitations i JOIN organizations o ON o.id=i.organization_id JOIN accounts a ON a.id=i.invited_by JOIN accounts me ON me.email=i.email WHERE me.id=$1 AND i.expires_at>now() AND ` + isInvitedByAdmin, "", "", nil
	case "/share-invitations":
		return shareInvitationCollection, "", "", nil
	case "/vault-key-requests":
		return keyRequestCollection, "", "", nil
	case "/admin/invites":
		return `SELECT encode(i.token_hash,'hex') AS key,'invites' AS field,'' AS parent,jsonb_build_object('id',encode(i.token_hash,'hex'),'email',i.email,'role',i.role,'createdBy',CASE WHEN a.id IS NULL THEN NULL ELSE jsonb_build_object('accountId',a.id,'name',a.name,'email',a.email) END,'createdAt',i.created_at,'expiresAt',i.expires_at) AS value FROM invites i LEFT JOIN accounts a ON a.id=i.created_by WHERE i.email<>'' AND i.used_at IS NULL AND i.expires_at>now() AND ` + isWithinCreatorAuthority, "", "", nil
	}
	if len(parts) == 3 && parts[0] == "vaults" && parts[2] == "members" {
		return `SELECT account_id AS key,'' AS field,'' AS parent,jsonb_build_object('accountId',account_id,'role',role,'grantedBy',granted_by) AS value FROM vault_access WHERE vault_id=$3::uuid`, parts[1], "", nil
	}
	if len(parts) == 5 && parts[0] == "vaults" && parts[2] == "items" && parts[4] == "history" {
		return historyCollectionPageSQL, parts[1], parts[3], nil
	}
	if len(parts) == 2 && parts[0] == "organizations" {
		return organizationCollection, parts[1], "", nil
	}
	if len(parts) == 3 && parts[0] == "admin" && parts[1] == "accounts" {
		return adminAccountCollection, parts[2], "", nil
	}
	return "", "", "", ErrNotFound
}

// ReadCollectionPage performs each page in a read-only repeatable snapshot. A transactional
// revision check rejects a mixture of pages after any relevant committed mutation.
func (s *Store) ReadCollectionPage(ctx context.Context, accountID, deviceID, path, encodedCursor string) (apitypes.CollectionPage, error) {
	var page apitypes.CollectionPage
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var err error
		page, err = readCollectionPage(ctx, tx, accountID, deviceID, path, encodedCursor)
		return err
	})
	return page, err
}

// ReadCollection hands visit every page of a collection from one read-only repeatable snapshot, for a server-side reader
// that assembles the whole collection: a concurrent write never invalidates it. visit stops the walk by returning an error.
func (s *Store) ReadCollection(ctx context.Context, accountID, deviceID, path string, visit func(apitypes.CollectionPage) error) error {
	return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		cursor := ""
		for {
			page, err := readCollectionPage(ctx, tx, accountID, deviceID, path, cursor)
			if err != nil {
				return err
			}
			if err := visit(page); err != nil {
				return err
			}
			if page.NextCursor == "" {
				return nil
			}
			cursor = page.NextCursor
		}
	})
}

func readCollectionPage(ctx context.Context, tx pgx.Tx, accountID, deviceID, path, encodedCursor string) (apitypes.CollectionPage, error) {
	page := apitypes.CollectionPage{Version: apitypes.CollectionPageVersion, Records: []apitypes.CollectionRecord{}}
	query, vaultOrOrganizationID, itemID, err := collectionQuery(path)
	if err != nil {
		return page, err
	}
	scope := accountID + "/" + deviceID + path
	cursor := collectionCursor{Scope: scope, At: time.Now().UTC()}
	if encodedCursor != "" {
		if len(encodedCursor) > 4096 {
			return page, ErrConflict
		}
		decoded, err := base64.RawURLEncoding.DecodeString(encodedCursor)
		if err != nil || json.Unmarshal(decoded, &cursor) != nil || cursor.Scope != scope || len(cursor.After) > 512 || cursor.At.IsZero() {
			return page, ErrConflict
		}
	}
	query = strings.ReplaceAll(query, "now()", "$7::timestamptz")
	if strings.HasPrefix(path, "/admin/") {
		var isAdmin bool
		if err := tx.QueryRow(ctx, `SELECT role='admin' AND suspended_at IS NULL FROM accounts WHERE id=$1`, accountID).Scan(&isAdmin); err != nil {
			return page, convertNoRowsToNotFound(err)
		}
		if !isAdmin {
			return page, ErrActorNotAdmin
		}
		if strings.HasPrefix(path, "/admin/accounts/") {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)`, vaultOrOrganizationID).Scan(&exists); err != nil {
				return page, err
			}
			if !exists {
				return page, ErrNotFound
			}
		}
	}
	if strings.HasPrefix(path, "/vaults/") {
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vault_access WHERE vault_id=$1 AND account_id=$2)`, vaultOrOrganizationID, accountID).Scan(&member); err != nil {
			return page, err
		}
		if !member {
			return page, ErrNotFound
		}
	}
	if strings.HasPrefix(path, "/organizations/") {
		if _, err := getOrganizationSummary(ctx, tx, vaultOrOrganizationID, accountID); err != nil {
			return page, err
		}
	}
	revision, err := collectionRevision(ctx, tx, accountID, path, vaultOrOrganizationID)
	if err != nil {
		return page, err
	}
	if encodedCursor != "" && revision != cursor.Revision {
		return page, ErrCollectionChanged
	}
	cursor.Revision = revision
	if itemID != "" && cursor.After != "" && uuid.Validate(cursor.After) != nil {
		return page, ErrConflict
	}
	var pageSQL string
	if itemID != "" {
		pageSQL = query
	} else {
		// Typed parameter CTEs keep unused parameters well defined for all collection queries.
		pageSQL = `WITH parameters AS (SELECT $1::text,$2::text,$3::text,$4::text,$7::timestamptz), source AS (` + query + `), candidates AS (
   SELECT key,jsonb_build_object('key',key,'field',field,'parent',parent,'value',value)::text AS record FROM source WHERE key COLLATE "C">$5 ORDER BY key COLLATE "C" LIMIT 101
  ), sized AS (SELECT *,sum(octet_length(record)) OVER(ORDER BY key COLLATE "C") AS bytes,row_number() OVER(ORDER BY key COLLATE "C") AS n FROM candidates)
  SELECT key,CASE WHEN bytes<=$6 AND n<=100 THEN record END FROM sized ORDER BY key COLLATE "C"`
	}
	rows, err := tx.Query(ctx, pageSQL, accountID, deviceID, vaultOrOrganizationID, itemID, cursor.After, apitypes.CollectionPageBytes-4096, cursor.At)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var record *string
		if err := rows.Scan(&key, &record); err != nil {
			return page, err
		}
		if record == nil {
			if len(page.Records) == 0 {
				return page, ErrCollectionTooLarge
			}
			data, _ := json.Marshal(cursor)
			page.NextCursor = base64.RawURLEncoding.EncodeToString(data)
			break
		}
		var value apitypes.CollectionRecord
		if err := json.Unmarshal([]byte(*record), &value); err != nil {
			return page, err
		}
		page.Records = append(page.Records, value)
		cursor.After = key
	}
	return page, rows.Err()
}

const organizationCollection = `
 SELECT '0' AS key,'$' AS field,'' AS parent,jsonb_build_object('id',o.id,'name',o.name,'role',m.role) AS value FROM organizations o JOIN organization_members m ON m.organization_id=o.id WHERE o.id=$3::uuid AND m.account_id=$1
 UNION ALL SELECT '1/'||to_char(m.added_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||a.id,'members','',jsonb_build_object('accountId',a.id,'email',a.email,'name',a.name,'role',m.role,'publicKeys',` + publicKeysJSON + `) FROM organization_members m JOIN accounts a ON a.id=m.account_id WHERE m.organization_id=$3::uuid
 UNION ALL SELECT '2/'||lower(name)||'/'||id::text,'teams','',jsonb_build_object('id',id,'name',name,'memberIds','[]'::jsonb,'vaults','[]'::jsonb) FROM teams WHERE organization_id=$3::uuid
 UNION ALL SELECT '3/'||team_id::text||'/'||to_char(added_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||account_id,'memberIds',team_id::text,to_jsonb(account_id) FROM team_members WHERE organization_id=$3::uuid
 UNION ALL SELECT '4/'||team_id::text||'/'||to_char(granted_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||vault_id::text,'vaults',team_id::text,jsonb_build_object('vaultId',vault_id,'role',role) FROM team_vaults WHERE organization_id=$3::uuid
 UNION ALL SELECT '5/'||to_char(i.created_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||i.id::text,'invitations','',jsonb_build_object('id',i.id,'email',i.email,'role',i.role,'invitedBy',i.invited_by,'createdAt',i.created_at,'expiresAt',i.expires_at) FROM organization_invitations i JOIN organization_members m ON m.organization_id=i.organization_id AND m.account_id=$1 AND m.role='admin' WHERE i.organization_id=$3::uuid AND i.expires_at>now() AND ` + isInvitedByAdmin

const shareInvitationCollection = `
 SELECT 'r/'||to_char(i.created_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||i.id::text AS key,'received' AS field,'' AS parent,jsonb_build_object('id',i.id,'vaultId',i.vault_id,'role',i.role,'invitedByName',a.name,'invitedByEmail',a.email,'createdAt',i.created_at,'expiresAt',i.expires_at) AS value FROM share_invitations i JOIN accounts a ON a.id=i.invited_by JOIN accounts me ON me.email=i.email WHERE me.id=$1 AND i.state='pending' AND i.expires_at>now() AND ` + isInvitedByOwner + `
 UNION ALL SELECT 's/'||to_char(i.created_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||i.id::text,'sent','',jsonb_build_object('id',i.id,'vaultId',i.vault_id,'email',i.email,'role',i.role,'state',i.state,'invitedBy',i.invited_by,'createdAt',i.created_at,'expiresAt',i.expires_at,'acceptedAt',i.accepted_at,'intent',to_jsonb(i)->'intent','recipient',CASE WHEN a.id IS NOT NULL THEN ` + memberJSON + ` ELSE NULL END) FROM share_invitations i JOIN vault_access owner ON owner.vault_id=i.vault_id AND owner.account_id=$1 AND owner.role='owner' LEFT JOIN accounts a ON a.id=i.accepted_by WHERE (i.state='accepted' OR i.expires_at>now()) AND ` + isInvitedByOwner + `
 UNION ALL SELECT 'w/'||to_char(i.accepted_at AT TIME ZONE 'UTC','YYYYMMDDHH24MISS.US')||'/'||i.id::text,'waiting','',jsonb_build_object('id',i.id,'vaultId',i.vault_id,'role',i.role,'invitedByName',a.name,'invitedByEmail',a.email,'acceptedAt',i.accepted_at) FROM share_invitations i JOIN accounts a ON a.id=i.invited_by WHERE i.accepted_by=$1 AND i.state='accepted' AND ` + isInvitedByOwner

const strongestRoleSQL = `CASE max(CASE tv.role WHEN 'owner' THEN 3 WHEN 'editor' THEN 2 ELSE 1 END) WHEN 3 THEN 'owner' WHEN 2 THEN 'editor' ELSE 'reader' END`
const keyRequestCollection = `
 SELECT 'p/'||v.id::text||'/'||a.email||'/'||a.id AS key,'pending' AS field,'' AS parent,jsonb_build_object('vaultId',v.id,'role',` + strongestRoleSQL + `,'keyGeneration',v.key_generation,'recipient',` + memberJSON + `) AS value
 FROM vault_access mine JOIN team_vaults tv ON tv.vault_id=mine.vault_id JOIN team_members tm ON tm.team_id=tv.team_id JOIN vaults v ON v.id=tv.vault_id JOIN accounts a ON a.id=tm.account_id
 WHERE mine.account_id=$1 AND mine.role='owner' AND NOT EXISTS(SELECT 1 FROM vault_access other WHERE other.vault_id=v.id AND other.account_id=a.id) GROUP BY v.id,a.id
 UNION ALL SELECT 'w/'||tv.vault_id::text,'waiting','',jsonb_build_object('vaultId',tv.vault_id,'organizationId',tv.organization_id,'role',` + strongestRoleSQL + `) FROM team_members tm JOIN team_vaults tv ON tv.team_id=tm.team_id WHERE tm.account_id=$1 AND NOT EXISTS(SELECT 1 FROM vault_access a WHERE a.vault_id=tv.vault_id AND a.account_id=$1) GROUP BY tv.vault_id,tv.organization_id`

func collectionRevision(ctx context.Context, tx pgx.Tx, accountID, path, entityID string) (string, error) {
	var revision string
	if strings.HasPrefix(path, "/vaults/") {
		return scopedCollectionRevision(ctx, tx, []string{"v/" + entityID})
	}
	if strings.HasPrefix(path, "/organizations/") {
		return scopedCollectionRevision(ctx, tx, []string{"o/" + entityID, "directory"})
	}
	if strings.HasPrefix(path, "/admin/accounts/") {
		return scopedCollectionRevision(ctx, tx, []string{"d/" + entityID, "av/" + entityID, "directory"})
	}
	visibleVaults, visibleOrganizations := false, false
	scopes := []string{}
	switch path {
	case "/vaults":
		visibleVaults = true
		scopes = []string{"av/" + accountID}
	case "/devices":
		scopes = []string{"d/" + accountID}
	case "/share-links":
		scopes = []string{"l/" + accountID}
	case "/members":
		visibleVaults = true
		scopes = []string{"av/" + accountID, "directory"}
	case "/organizations":
		visibleOrganizations = true
		scopes = []string{"ao/" + accountID, "directory"}
	case "/share-invitations":
		visibleVaults = true
		scopes = []string{"av/" + accountID, "directory"}
	case "/vault-key-requests":
		visibleVaults, visibleOrganizations = true, true
		scopes = []string{"av/" + accountID, "ao/" + accountID, "directory"}
	case "/admin/invites":
		scopes = []string{"admin-invites", "directory"}
	}
	if path == "/organizations" || path == "/share-invitations" {
		var email string
		if err := tx.QueryRow(ctx, `SELECT email FROM accounts WHERE id=$1`, accountID).Scan(&email); err != nil {
			return "", err
		}
		scopes = append(scopes, "i/"+email)
	}
	err := tx.QueryRow(ctx, `WITH visible_scopes AS (
	 SELECT 'v/'||vault_id::text AS scope FROM vault_access WHERE account_id=$1 AND $2::boolean
	 UNION SELECT 'o/'||organization_id::text FROM organization_members WHERE account_id=$1 AND $3::boolean
	)
	SELECT jsonb_build_array(
	 (SELECT coalesce(sum(revision),0) FROM collection_versions WHERE scope IN(SELECT scope FROM visible_scopes)),
	 (SELECT jsonb_agg(jsonb_build_array(coalesce(v.revision,0),v.incarnation) ORDER BY s.ordinal)
	  FROM unnest($4::text[]) WITH ORDINALITY AS s(scope,ordinal)
	  LEFT JOIN collection_versions v ON v.scope=s.scope)
	)::text`, accountID, visibleVaults, visibleOrganizations, scopes).Scan(&revision)
	return revision, err
}

func scopedCollectionRevision(ctx context.Context, tx pgx.Tx, scopes []string) (string, error) {
	var revision string
	err := tx.QueryRow(ctx, `SELECT jsonb_agg(jsonb_build_array(coalesce(v.revision,0),v.incarnation) ORDER BY s.ordinal)::text
	 FROM unnest($1::text[]) WITH ORDINALITY AS s(scope,ordinal)
	 LEFT JOIN collection_versions v ON v.scope=s.scope`, scopes).Scan(&revision)
	return revision, err
}

const adminAccountCollection = `
 SELECT '0' AS key,'account' AS field,'' AS parent,jsonb_build_object('id',a.id,'email',a.email,'name',a.name,'role',a.role,'state',CASE WHEN a.suspended_at IS NULL THEN 'active' ELSE 'suspended' END,'createdAt',a.created_at,'lastSeenAt',(SELECT max(d.last_seen_at) FROM devices d WHERE d.account_id=a.id),'deviceCount',(SELECT count(*) FROM devices d WHERE d.account_id=a.id AND d.revoked_at IS NULL),'vaultCount',(SELECT count(*) FROM vault_access v WHERE v.account_id=a.id)) AS value FROM accounts a WHERE a.id=$3
 UNION ALL SELECT '1/'||id::text,'devices','',jsonb_build_object('id',id,'name',name,'platform',platform,'createdAt',created_at,'lastSeenAt',last_seen_at,'revokedAt',revoked_at) FROM devices WHERE account_id=$3`

// Size the raw ciphertext before base64 encoding it. Two bytes per ciphertext byte plus
// 1 KiB metadata bounds JSON without constructing oversized lookahead records in Postgres.
const historyCollectionPageSQL = `
 WITH parameters AS(SELECT $1::text,$2::text,$7::timestamptz), candidates AS(
  SELECT *,sum(2*(octet_length(overview)::bigint+octet_length(details))+1024) OVER(ORDER BY revision) AS bytes,
   row_number() OVER(ORDER BY revision) AS n
  FROM item_history WHERE vault_id=$3::uuid AND id=$4::uuid AND ($5='' OR revision>NULLIF($5,'')::uuid)
  ORDER BY revision LIMIT 101
 )
 SELECT revision::text,CASE WHEN bytes<=$6 AND n<=100 THEN
  jsonb_build_object('key',revision::text,'field','','parent','','value',
   jsonb_build_object('id',id,'revision',revision,'seq',0,'deleted',deleted,'overview',encode(overview,'base64'),'details',encode(details,'base64'),'updatedAt',updated_at,'updatedBy',updated_by))::text
 END FROM candidates ORDER BY revision`
