-- Uma instância Evolution Go só pode pertencer a uma inbox/tenant.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_evo_instance_name_unique
    ON tenants (evo_instance_name);
