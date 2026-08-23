// Package dbroles configures login secrets for the fixed database roles that
// are created and validated by the public role-separation migration.
package dbroles

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// LookupFunc resolves bootstrap configuration without coupling the package to
// process-global environment state.
type LookupFunc func(string) (string, bool)

// Config contains the one-time administrator connection and operational role
// secrets. It must never be passed to the API or provisioner.
type Config struct {
	BootstrapDatabaseURL string
	RuntimePassword      string
	MigratorPassword     string
	ProvisionerPassword  string
}

// LoadConfig reads role-bootstrap settings while keeping secret values out of
// validation errors.
func LoadConfig(lookup LookupFunc) (Config, error) {
	config := Config{
		BootstrapDatabaseURL: value(lookup, "BOOTSTRAP_DATABASE_URL"),
		RuntimePassword:      value(lookup, "DB_RUNTIME_PASSWORD"),
		MigratorPassword:     value(lookup, "DB_MIGRATOR_PASSWORD"),
		ProvisionerPassword:  value(lookup, "DB_PROVISIONER_PASSWORD"),
	}
	if err := config.validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) validate() error {
	missing := make([]string, 0, 4)
	for _, setting := range []struct {
		name  string
		value string
	}{
		{"BOOTSTRAP_DATABASE_URL", c.BootstrapDatabaseURL},
		{"DB_RUNTIME_PASSWORD", c.RuntimePassword},
		{"DB_MIGRATOR_PASSWORD", c.MigratorPassword},
		{"DB_PROVISIONER_PASSWORD", c.ProvisionerPassword},
	} {
		if strings.TrimSpace(setting.value) == "" {
			missing = append(missing, setting.name)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("database role bootstrap requires %s", strings.Join(missing, ", "))
	}
	return nil
}

// Configure verifies the migration-owned role topology and assigns the three
// login passwords in one transaction. Passwords travel as query parameters and
// never appear in the SQL statement or returned errors.
func Configure(ctx context.Context, config Config) error {
	if err := config.validate(); err != nil {
		return err
	}
	connectionConfig, err := pgx.ParseConfig(config.BootstrapDatabaseURL)
	if err != nil {
		return errors.New("BOOTSTRAP_DATABASE_URL is invalid")
	}
	connection, err := pgx.ConnectConfig(ctx, connectionConfig)
	if err != nil {
		return errors.New("could not connect with BOOTSTRAP_DATABASE_URL")
	}
	defer func() { _ = connection.Close(context.Background()) }()

	tx, err := connection.Begin(ctx)
	if err != nil {
		return errors.New("could not start database role bootstrap")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	for _, role := range []struct {
		name     string
		canLogin bool
	}{
		{"synodus_owner", false},
		{"synodus_migrator", true},
		{"synodus_provisioner", true},
		{"synodus_runtime", true},
	} {
		var canLogin, superuser, createRole, createDB, inherit, replication, bypassRLS bool
		err = tx.QueryRow(ctx, `
			SELECT rolcanlogin, rolsuper, rolcreaterole, rolcreatedb, rolinherit,
			       rolreplication, rolbypassrls
			FROM pg_roles
			WHERE rolname = $1`, role.name).Scan(
			&canLogin, &superuser, &createRole, &createDB, &inherit, &replication, &bypassRLS,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("database roles are missing; apply public migrations with bootstrap credentials first")
		}
		if err != nil {
			return errors.New("could not verify database role topology")
		}
		if canLogin != role.canLogin || superuser || createRole || createDB || inherit || replication || bypassRLS {
			return fmt.Errorf("refusing to configure unexpected attributes for database role %s", role.name)
		}
	}

	_, err = tx.Exec(ctx, `
		SELECT set_config('synodus.bootstrap_runtime_password', $1, true),
		       set_config('synodus.bootstrap_migrator_password', $2, true),
		       set_config('synodus.bootstrap_provisioner_password', $3, true)`,
		config.RuntimePassword, config.MigratorPassword, config.ProvisionerPassword,
	)
	if err != nil {
		return errors.New("could not stage database role passwords")
	}
	_, err = tx.Exec(ctx, `
		DO $bootstrap$
		BEGIN
			EXECUTE format('ALTER ROLE synodus_runtime PASSWORD %L',
				current_setting('synodus.bootstrap_runtime_password'));
			EXECUTE format('ALTER ROLE synodus_migrator PASSWORD %L',
				current_setting('synodus.bootstrap_migrator_password'));
			EXECUTE format('ALTER ROLE synodus_provisioner PASSWORD %L',
				current_setting('synodus.bootstrap_provisioner_password'));
			PERFORM set_config('synodus.bootstrap_runtime_password', '', true);
			PERFORM set_config('synodus.bootstrap_migrator_password', '', true);
			PERFORM set_config('synodus.bootstrap_provisioner_password', '', true);
		END
		$bootstrap$`)
	if err != nil {
		return errors.New("could not assign database role passwords")
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.New("could not commit database role bootstrap")
	}
	return nil
}

func value(lookup LookupFunc, name string) string {
	value, _ := lookup(name)
	return strings.TrimSpace(value)
}
