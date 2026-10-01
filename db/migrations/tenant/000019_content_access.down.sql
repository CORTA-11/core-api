DROP TRIGGER file_content_owner ON files;
DROP TRIGGER document_content_owner ON documents;
DROP FUNCTION synodus_register_content_owner();
DROP FUNCTION synodus_can_access_content(TEXT, UUID);
DROP TABLE content_access_requests;
DROP TABLE content_owners;
