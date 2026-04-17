package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/servers"
)

type ServerRegistryRepository struct {
	pool *pgxpool.Pool
}

func NewServerRegistryRepository(pool *pgxpool.Pool) *ServerRegistryRepository {
	return &ServerRegistryRepository{pool: pool}
}

func (r *ServerRegistryRepository) Create(ctx context.Context, s servers.Server, encryptedSecret, encryptedDEK []byte) (servers.Server, error) {
	if r == nil || r.pool == nil {
		return servers.Server{}, fmt.Errorf("timescale not configured")
	}
	s.Name = strings.TrimSpace(s.Name)
	s.Host = strings.TrimSpace(s.Host)
	s.Username = strings.TrimSpace(s.Username)
	if s.Name == "" || s.Host == "" || s.Username == "" {
		return servers.Server{}, fmt.Errorf("invalid server")
	}

	// Use DB default UUID; return it.
	var id uuid.UUID
	var createdAt, updatedAt time.Time
	err := r.pool.QueryRow(ctx, `
		INSERT INTO optima_servers
			(name, db_type, host, port, username, auth_type, encrypted_secret, encrypted_dek, ssl_mode, is_active, created_by)
		VALUES
			($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at, updated_at
	`,
		s.Name, string(s.DBType), s.Host, s.Port, s.Username, string(s.AuthType),
		encryptedSecret, encryptedDEK, string(s.SSLMode), s.IsActive, strings.TrimSpace(s.CreatedBy),
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return servers.Server{}, err
	}
	s.ID = id.String()
	s.CreatedAt = createdAt
	s.UpdatedAt = updatedAt
	return s, nil
}

func (r *ServerRegistryRepository) List(ctx context.Context, activeOnly bool) ([]servers.Server, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	q := `
		SELECT id, name, db_type, host, port, username, auth_type, ssl_mode, is_active, created_at, updated_at, COALESCE(created_by,''), last_test_at
		FROM optima_servers
	`
	args := []any{}
	if activeOnly {
		q += " WHERE is_active = TRUE"
	}
	q += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []servers.Server
	for rows.Next() {
		var id uuid.UUID
		var s servers.Server
		var dbType, authType, sslMode string
		var lastTest sql.NullTime
		if err := rows.Scan(&id, &s.Name, &dbType, &s.Host, &s.Port, &s.Username, &authType, &sslMode, &s.IsActive, &s.CreatedAt, &s.UpdatedAt, &s.CreatedBy, &lastTest); err != nil {
			continue
		}
		s.ID = id.String()
		s.DBType = servers.DBType(dbType)
		s.AuthType = servers.AuthType(authType)
		s.SSLMode = servers.SSLMode(sslMode)
		if lastTest.Valid {
			t := lastTest.Time.UTC()
			s.LastTestAt = &t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ServerRegistryRepository) GetByName(ctx context.Context, name string) (servers.Server, error) {
	if r == nil || r.pool == nil {
		return servers.Server{}, fmt.Errorf("timescale not configured")
	}
	var s servers.Server
	var id uuid.UUID
	var dbType, authType, sslMode string
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, db_type, host, port, username, auth_type, ssl_mode, is_active, created_at, updated_at
		FROM optima_servers
		WHERE name = $1 AND is_active = TRUE
		LIMIT 1
	`, strings.TrimSpace(name)).Scan(&id, &s.Name, &dbType, &s.Host, &s.Port, &s.Username, &authType, &sslMode, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return servers.Server{}, err
	}
	s.ID = id.String()
	s.DBType = servers.DBType(dbType)
	s.AuthType = servers.AuthType(authType)
	s.SSLMode = servers.SSLMode(sslMode)
	return s, nil
}

func (r *ServerRegistryRepository) GetEncrypted(ctx context.Context, id string) (servers.Server, []byte, []byte, error) {
	if r == nil || r.pool == nil {
		return servers.Server{}, nil, nil, fmt.Errorf("timescale not configured")
	}
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return servers.Server{}, nil, nil, fmt.Errorf("invalid server id")
	}

	var s servers.Server
	var dbType, authType, sslMode string
	var encSecret, encDEK []byte
	var createdBy string
	err = r.pool.QueryRow(ctx, `
		SELECT name, db_type, host, port, username, auth_type, encrypted_secret, encrypted_dek, ssl_mode, is_active, created_at, updated_at, COALESCE(created_by,'')
		FROM optima_servers
		WHERE id = $1
	`, uid).Scan(&s.Name, &dbType, &s.Host, &s.Port, &s.Username, &authType, &encSecret, &encDEK, &sslMode, &s.IsActive, &s.CreatedAt, &s.UpdatedAt, &createdBy)
	if err != nil {
		return servers.Server{}, nil, nil, err
	}
	s.ID = uid.String()
	s.DBType = servers.DBType(dbType)
	s.AuthType = servers.AuthType(authType)
	s.SSLMode = servers.SSLMode(sslMode)
	s.CreatedBy = createdBy
	return s, encSecret, encDEK, nil
}

func (r *ServerRegistryRepository) Delete(ctx context.Context, id string) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("invalid server id")
	}
	ct, err := r.pool.Exec(ctx, `DELETE FROM optima_servers WHERE id=$1`, uid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("server not found")
	}
	return nil
}

func (r *ServerRegistryRepository) SetActive(ctx context.Context, id string, active bool) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("invalid server id")
	}
	_, err = r.pool.Exec(ctx, `UPDATE optima_servers SET is_active=$2, updated_at=now() WHERE id=$1`, uid, active)
	return err
}

func (r *ServerRegistryRepository) UpdateMetadata(ctx context.Context, id string, name, host string, port int, username, sslMode string) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("invalid server id")
	}
	ct, err := r.pool.Exec(ctx, `
		UPDATE optima_servers SET name=$2, host=$3, port=$4, username=$5, ssl_mode=$6, updated_at=now()
		WHERE id=$1
	`, uid, strings.TrimSpace(name), strings.TrimSpace(host), port, strings.TrimSpace(username), strings.TrimSpace(sslMode))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("server not found")
	}
	return nil
}

func (r *ServerRegistryRepository) UpdateCredentials(ctx context.Context, id string, encryptedSecret, encryptedDEK []byte) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("invalid server id")
	}
	ct, err := r.pool.Exec(ctx, `
		UPDATE optima_servers SET encrypted_secret=$2, encrypted_dek=$3, updated_at=now() WHERE id=$1
	`, uid, encryptedSecret, encryptedDEK)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("server not found")
	}
	return nil
}

func (r *ServerRegistryRepository) TouchLastTest(ctx context.Context, id string, at time.Time) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("invalid server id")
	}
	_, err = r.pool.Exec(ctx, `UPDATE optima_servers SET last_test_at=$2, updated_at=now() WHERE id=$1`, uid, at.UTC())
	return err
}
