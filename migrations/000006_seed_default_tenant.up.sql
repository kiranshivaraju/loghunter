INSERT INTO tenants (name, loki_org_id)
VALUES ('default', 'default')
ON CONFLICT DO NOTHING;
