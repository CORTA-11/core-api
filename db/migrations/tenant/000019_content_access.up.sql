-- Immutable ownership is kept apart from the document's last editor.
CREATE TABLE content_owners (
    kind TEXT NOT NULL CHECK (kind IN ('file', 'document')),
    resource_id UUID NOT NULL,
    team_id BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    creator_id UUID NOT NULL REFERENCES public.users(user_id),
    PRIMARY KEY (kind, resource_id)
);
CREATE INDEX content_owners_team_idx ON content_owners(team_id, kind, resource_id);
CREATE TABLE content_access_requests (
    public_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind TEXT NOT NULL,
    resource_id UUID NOT NULL,
    requested_by UUID NOT NULL REFERENCES public.users(user_id),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'granted', 'denied')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kind, resource_id, requested_by),
    FOREIGN KEY (kind, resource_id) REFERENCES content_owners(kind, resource_id) ON DELETE CASCADE
);
-- Older documents did not record their creator; their last editor becomes the
-- initial owner at cutover. New documents always retain their original creator.
INSERT INTO content_owners SELECT 'document', public_id, team_id, last_updated_by FROM documents;
INSERT INTO content_owners SELECT 'file', public_id, team_id, uploaded_by FROM files WHERE deleted_at IS NULL;

CREATE FUNCTION synodus_register_content_owner() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        DELETE FROM content_owners WHERE kind = TG_ARGV[0] AND resource_id = OLD.public_id;
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.deleted_at IS NOT NULL THEN
            DELETE FROM content_owners WHERE kind = TG_ARGV[0] AND resource_id = NEW.public_id;
        END IF;
        RETURN NEW;
    END IF;
    INSERT INTO content_owners(kind, resource_id, team_id, creator_id)
    VALUES (TG_ARGV[0], NEW.public_id, NEW.team_id, (to_jsonb(NEW)->>TG_ARGV[1])::uuid);
    RETURN NEW;
END;
$$;
CREATE TRIGGER document_content_owner AFTER INSERT OR DELETE ON documents
FOR EACH ROW EXECUTE FUNCTION synodus_register_content_owner('document', 'last_updated_by');
CREATE TRIGGER file_content_owner AFTER INSERT OR DELETE OR UPDATE OF deleted_at ON files
FOR EACH ROW EXECUTE FUNCTION synodus_register_content_owner('file', 'uploaded_by');

ALTER TABLE content_owners OWNER TO synodus_owner;
ALTER TABLE content_access_requests OWNER TO synodus_owner;
ALTER FUNCTION synodus_register_content_owner() OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION synodus_register_content_owner() FROM PUBLIC;
ALTER TABLE content_owners ENABLE ROW LEVEL SECURITY;
ALTER TABLE content_owners FORCE ROW LEVEL SECURITY;
ALTER TABLE content_access_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE content_access_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY content_owners_maintenance ON content_owners FOR ALL TO synodus_owner USING (true) WITH CHECK (true);
CREATE POLICY content_requests_maintenance ON content_access_requests FOR ALL TO synodus_owner USING (true) WITH CHECK (true);
CREATE POLICY content_owners_read ON content_owners FOR SELECT TO synodus_runtime USING (
    team_id = NULLIF(current_setting('app.team_id', true), '')::bigint AND synodus_has_team_membership(team_id)
);
CREATE POLICY content_requests_read ON content_access_requests FOR SELECT TO synodus_runtime USING (
    EXISTS (SELECT 1 FROM content_owners o WHERE o.kind = content_access_requests.kind
        AND o.resource_id = content_access_requests.resource_id
        AND (o.creator_id = synodus_app_user_public_id() OR requested_by = synodus_app_user_public_id()))
);
CREATE POLICY content_requests_insert ON content_access_requests FOR INSERT TO synodus_runtime WITH CHECK (
    EXISTS (SELECT 1 FROM content_owners o WHERE o.kind = content_access_requests.kind AND o.resource_id = content_access_requests.resource_id
        AND ((requested_by = synodus_app_user_public_id() AND status = 'pending')
             OR (o.creator_id = synodus_app_user_public_id() AND status = 'granted')))
);
CREATE POLICY content_requests_update ON content_access_requests FOR UPDATE TO synodus_runtime USING (
    EXISTS (SELECT 1 FROM content_owners o WHERE o.kind = content_access_requests.kind AND o.resource_id = content_access_requests.resource_id
        AND (o.creator_id = synodus_app_user_public_id() OR requested_by = synodus_app_user_public_id()))
) WITH CHECK (
    EXISTS (SELECT 1 FROM content_owners o WHERE o.kind = content_access_requests.kind AND o.resource_id = content_access_requests.resource_id
        AND (o.creator_id = synodus_app_user_public_id() OR (requested_by = synodus_app_user_public_id() AND status = 'pending')))
);
GRANT SELECT ON content_owners TO synodus_runtime;
GRANT SELECT, INSERT, UPDATE ON content_access_requests TO synodus_runtime;

CREATE FUNCTION synodus_can_access_content(candidate_kind TEXT, candidate_id UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE SECURITY INVOKER SET search_path FROM CURRENT AS $$
    SELECT EXISTS (SELECT 1 FROM content_owners o WHERE o.kind = candidate_kind AND o.resource_id = candidate_id
        AND (o.creator_id = synodus_app_user_public_id() OR EXISTS (
            SELECT 1 FROM content_access_requests r WHERE r.kind = o.kind AND r.resource_id = o.resource_id
                AND r.requested_by = synodus_app_user_public_id() AND r.status = 'granted')));
$$;
ALTER FUNCTION synodus_can_access_content(TEXT, UUID) OWNER TO synodus_owner;
REVOKE ALL ON FUNCTION synodus_can_access_content(TEXT, UUID) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION synodus_can_access_content(TEXT, UUID) TO synodus_runtime;
