CREATE INDEX idx_namespaces_active_depth_path
    ON namespaces (depth, path)
    WHERE deleted_at IS NULL;
