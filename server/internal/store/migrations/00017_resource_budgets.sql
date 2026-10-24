-- +goose Up
CREATE TABLE collection_versions (
 scope text PRIMARY KEY,
 revision bigint NOT NULL DEFAULT 1,
 incarnation uuid NOT NULL DEFAULT gen_random_uuid()
);
CREATE TABLE resource_usage (
 resource text NOT NULL,
 scope text NOT NULL,
 used bigint NOT NULL CHECK (used>=0),
 PRIMARY KEY(resource,scope)
);

-- Rows in this queue are private to one transaction and disappear in its deferred flush.
CREATE TABLE resource_pending (
 transaction_id bigint NOT NULL,
 resource text NOT NULL,
 scope text NOT NULL,
 delta bigint NOT NULL DEFAULT 0,
 capacity bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(transaction_id,resource,scope)
);

-- +goose StatementBegin
CREATE FUNCTION bump_collection_version(scope_key text) RETURNS void LANGUAGE sql AS $$
 INSERT INTO resource_pending(transaction_id,resource,scope) VALUES(txid_current(),'',scope_key)
 ON CONFLICT(transaction_id,resource,scope) DO NOTHING;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION queue_resource_usage(resource_name text,scope_key text,change bigint,capacity_limit bigint) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF scope_key IS NULL OR change=0 THEN RETURN; END IF;
 INSERT INTO resource_pending(transaction_id,resource,scope,delta,capacity)
 VALUES(txid_current(),resource_name,scope_key,change,capacity_limit)
 ON CONFLICT(transaction_id,resource,scope) DO UPDATE SET delta=resource_pending.delta+EXCLUDED.delta;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION adjust_resource_usage(resource_name text,scope_key text,change bigint,capacity_limit bigint) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF change=0 THEN RETURN; END IF;
 -- A scope without a row has used nothing, and the conflict branch below never sees its first growth.
 IF change>capacity_limit THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='resource growth budget exceeded',DETAIL=resource_name; END IF;
 INSERT INTO resource_usage(resource,scope,used) VALUES(resource_name,scope_key,greatest(change,0))
 ON CONFLICT(resource,scope) DO UPDATE SET used=resource_usage.used+change
 WHERE change<=0 OR resource_usage.used+change<=capacity_limit;
 IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='resource growth budget exceeded',DETAIL=resource_name; END IF;
 DELETE FROM resource_usage WHERE resource=resource_name AND scope=scope_key AND used=0;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION resource_scope_retired(scope_key text) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT CASE
 WHEN scope_key LIKE 'v/%' THEN NOT EXISTS(SELECT 1 FROM vaults WHERE id=substr(scope_key,3)::uuid)
 WHEN scope_key LIKE 'o/%' THEN NOT EXISTS(SELECT 1 FROM organizations WHERE id=substr(scope_key,3)::uuid)
 WHEN scope_key LIKE 'av/%' OR scope_key LIKE 'ao/%' THEN NOT EXISTS(SELECT 1 FROM accounts WHERE id=substr(scope_key,4))
 WHEN scope_key LIKE 'd/%' OR scope_key LIKE 'l/%' THEN NOT EXISTS(SELECT 1 FROM accounts WHERE id=substr(scope_key,3))
 WHEN scope_key LIKE 'i/%' THEN NOT EXISTS(SELECT 1 FROM share_invitations WHERE email=substr(scope_key,3))
  AND NOT EXISTS(SELECT 1 FROM organization_invitations WHERE email=substr(scope_key,3))
 ELSE false END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION flush_resource_pending() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE transaction_key bigint:=txid_current(); entry record;
BEGIN
 -- All entity work is finished before any shared policy row is locked. Every flush uses the same order.
 FOR entry IN SELECT resource,scope,delta,capacity FROM resource_pending
  WHERE transaction_id=transaction_key AND resource<>'' AND delta<>0 ORDER BY resource,scope LOOP
  PERFORM adjust_resource_usage(entry.resource,entry.scope,entry.delta,entry.capacity);
 END LOOP;
 FOR entry IN SELECT scope FROM resource_pending
  WHERE transaction_id=transaction_key AND resource='' ORDER BY scope LOOP
  IF resource_scope_retired(entry.scope) THEN
   DELETE FROM collection_versions WHERE scope=entry.scope;
  ELSE
   INSERT INTO collection_versions(scope) VALUES(entry.scope)
   ON CONFLICT(scope) DO UPDATE SET revision=collection_versions.revision+1;
  END IF;
 END LOOP;
 DELETE FROM resource_pending WHERE transaction_id=transaction_key;
 RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER resource_pending_flush AFTER INSERT ON resource_pending
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION flush_resource_pending();

-- +goose StatementBegin
CREATE FUNCTION resource_collection_changed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r jsonb; previous jsonb; account_key text;
BEGIN
 r:=CASE WHEN TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
 IF TG_OP='UPDATE' THEN
  previous:=to_jsonb(OLD);
  IF TG_TABLE_NAME='devices' AND (r-'last_seen_at')=(previous-'last_seen_at') THEN RETURN NULL; END IF;
  IF TG_TABLE_NAME='accounts' AND
   (r->>'email',r->>'name',r->>'role',r->>'kem_pub',r->>'sign_pub',r->>'kem_sig',r->>'suspended_at') IS NOT DISTINCT FROM
   (previous->>'email',previous->>'name',previous->>'role',previous->>'kem_pub',previous->>'sign_pub',previous->>'kem_sig',previous->>'suspended_at') THEN RETURN NULL; END IF;
 END IF;
 CASE TG_TABLE_NAME
 WHEN 'accounts' THEN
  PERFORM bump_collection_version('directory');
  IF TG_OP='DELETE' THEN
   account_key:=r->>'id';
   PERFORM bump_collection_version('av/'||account_key);
   PERFORM bump_collection_version('ao/'||account_key);
   PERFORM bump_collection_version('d/'||account_key);
   PERFORM bump_collection_version('l/'||account_key);
  END IF;
 WHEN 'invites' THEN PERFORM bump_collection_version('admin-invites');
 WHEN 'devices' THEN PERFORM bump_collection_version('d/'||(r->>'account_id'));
 WHEN 'share_links' THEN PERFORM bump_collection_version('l/'||(r->>'account_id'));
 WHEN 'vaults' THEN PERFORM bump_collection_version('v/'||(r->>'id'));
 WHEN 'vault_access' THEN
  PERFORM bump_collection_version('v/'||(r->>'vault_id'));
  PERFORM bump_collection_version('av/'||(r->>'account_id'));
 WHEN 'items','item_history' THEN PERFORM bump_collection_version('v/'||(r->>'vault_id'));
 WHEN 'organizations' THEN PERFORM bump_collection_version('o/'||(r->>'id'));
 WHEN 'organization_members' THEN
  PERFORM bump_collection_version('o/'||(r->>'organization_id'));
  PERFORM bump_collection_version('ao/'||(r->>'account_id'));
 WHEN 'teams','team_members','team_vaults' THEN PERFORM bump_collection_version('o/'||(r->>'organization_id'));
 WHEN 'organization_invitations' THEN
  PERFORM bump_collection_version('o/'||(r->>'organization_id'));
  PERFORM bump_collection_version('i/'||(r->>'email'));
 WHEN 'share_invitations' THEN
  PERFORM bump_collection_version('v/'||(r->>'vault_id'));
  PERFORM bump_collection_version('i/'||(r->>'email'));
 END CASE;
 RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION resource_count_changed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE before_scope text; after_scope text; capacity_limit bigint:=TG_ARGV[2]::bigint;
BEGIN
 IF TG_OP<>'INSERT' THEN before_scope:=to_jsonb(OLD)->>TG_ARGV[1]; END IF;
 IF TG_OP<>'DELETE' THEN after_scope:=to_jsonb(NEW)->>TG_ARGV[1]; END IF;
 IF TG_TABLE_NAME='devices' THEN
  IF TG_OP<>'INSERT' AND OLD.revoked_at IS NOT NULL THEN before_scope:=NULL; END IF;
  IF TG_OP<>'DELETE' AND NEW.revoked_at IS NOT NULL THEN after_scope:=NULL; END IF;
 END IF;
 IF before_scope IS NOT DISTINCT FROM after_scope THEN RETURN NULL; END IF;
 PERFORM queue_resource_usage(TG_ARGV[0],before_scope,-1,capacity_limit);
 PERFORM queue_resource_usage(TG_ARGV[0],after_scope,1,capacity_limit);
 RETURN NULL;
END;
$$;
-- +goose StatementEnd

INSERT INTO resource_usage SELECT 'vaults',created_by,count(*) FROM vaults GROUP BY created_by;
INSERT INTO resource_usage SELECT 'devices',account_id,count(*) FROM devices WHERE revoked_at IS NULL GROUP BY account_id;
INSERT INTO resource_usage SELECT 'organizations',created_by,count(*) FROM organizations GROUP BY created_by;
INSERT INTO resource_usage SELECT 'teams',organization_id::text,count(*) FROM teams GROUP BY organization_id;
INSERT INTO resource_usage SELECT 'organization members',organization_id::text,count(*) FROM organization_members GROUP BY organization_id;
INSERT INTO resource_usage SELECT 'team memberships',organization_id::text,count(*) FROM team_members GROUP BY organization_id;
INSERT INTO resource_usage SELECT 'team grants',organization_id::text,count(*) FROM team_vaults GROUP BY organization_id;
INSERT INTO resource_usage SELECT 'vault grants',vault_id::text,count(*) FROM vault_access GROUP BY vault_id;

-- +goose StatementBegin
DO $$
DECLARE table_name text;
BEGIN
 FOREACH table_name IN ARRAY ARRAY['accounts','devices','share_links','vaults','vault_access','items','item_history','organizations','organization_members','teams','team_members','team_vaults','organization_invitations','share_invitations','invites'] LOOP
  EXECUTE format('CREATE TRIGGER resource_collection_version AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION resource_collection_changed()',table_name);
 END LOOP;
END $$;
-- +goose StatementEnd

CREATE TRIGGER resource_count AFTER INSERT OR UPDATE OR DELETE ON vaults FOR EACH ROW EXECUTE FUNCTION resource_count_changed('vaults','created_by','1024');
CREATE TRIGGER resource_count AFTER INSERT OR UPDATE OR DELETE ON devices FOR EACH ROW EXECUTE FUNCTION resource_count_changed('devices','account_id','256');
CREATE TRIGGER resource_count AFTER INSERT OR UPDATE OR DELETE ON organizations FOR EACH ROW EXECUTE FUNCTION resource_count_changed('organizations','created_by','128');
CREATE TRIGGER resource_count AFTER INSERT OR UPDATE OR DELETE ON teams FOR EACH ROW EXECUTE FUNCTION resource_count_changed('teams','organization_id','1024');
CREATE TRIGGER resource_count AFTER INSERT OR UPDATE OR DELETE ON organization_members FOR EACH ROW EXECUTE FUNCTION resource_count_changed('organization members','organization_id','4096');
CREATE TRIGGER resource_count AFTER INSERT OR UPDATE OR DELETE ON team_members FOR EACH ROW EXECUTE FUNCTION resource_count_changed('team memberships','organization_id','32768');
CREATE TRIGGER resource_count AFTER INSERT OR UPDATE OR DELETE ON team_vaults FOR EACH ROW EXECUTE FUNCTION resource_count_changed('team grants','organization_id','16384');
CREATE TRIGGER resource_count AFTER INSERT OR UPDATE OR DELETE ON vault_access FOR EACH ROW EXECUTE FUNCTION resource_count_changed('vault grants','vault_id','4096');

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE table_name text;
BEGIN
 FOREACH table_name IN ARRAY ARRAY['accounts','devices','share_links','vaults','vault_access','items','item_history','organizations','organization_members','teams','team_members','team_vaults','organization_invitations','share_invitations','invites'] LOOP
  EXECUTE format('DROP TRIGGER resource_collection_version ON %I',table_name);
 END LOOP;
 FOREACH table_name IN ARRAY ARRAY['vaults','devices','organizations','teams','organization_members','team_members','team_vaults','vault_access'] LOOP
  EXECUTE format('DROP TRIGGER resource_count ON %I',table_name);
 END LOOP;
END $$;
-- +goose StatementEnd
DROP TRIGGER resource_pending_flush ON resource_pending;
DROP FUNCTION resource_count_changed();
DROP FUNCTION resource_collection_changed();
DROP FUNCTION flush_resource_pending();
DROP FUNCTION resource_scope_retired(text);
DROP FUNCTION adjust_resource_usage(text,text,bigint,bigint);
DROP FUNCTION queue_resource_usage(text,text,bigint,bigint);
DROP FUNCTION bump_collection_version(text);
DROP TABLE resource_pending;
DROP TABLE resource_usage;
DROP TABLE collection_versions;
