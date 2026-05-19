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
	s.ID = id
	s.CreatedAt = createdAt
	s.UpdatedAt = updatedAt
	return s, nil
}

func (r *ServerRegistryRepository) List(ctx context.Context, activeOnly bool) ([]servers.Server, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("timescale not configured")
	}
	q := `
		SELECT  id, name, db_type, host, port, username, auth_type, ssl_mode, is_active, created_at, updated_at, COALESCE(created_by,''), last_test_at
		FROM optima_servers
		WHERE name IS NOT NULL AND name != '' AND name != 'undefined'
	`
	args := []any{}
	if activeOnly {
		q += " AND is_active = TRUE"
	}
	q += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []servers.Server
	for rows.Next() {
		var s servers.Server
		var dbType, authType, sslMode string
		var lastTest sql.NullTime
		if err := rows.Scan(&s.ID, &s.Name, &dbType, &s.Host, &s.Port, &s.Username, &authType, &sslMode, &s.IsActive, &s.CreatedAt, &s.UpdatedAt, &s.CreatedBy, &lastTest); err != nil {
			continue
		}
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
	var dbType, authType, sslMode string
	var createdBy string
	var lastTest sql.NullTime
	err := r.pool.QueryRow(ctx, `
		SELECT  id, name, db_type, host, port, username, auth_type, ssl_mode, is_active, created_at, updated_at, COALESCE(created_by,''), last_test_at
		FROM optima_servers
		WHERE UPPER(name) = UPPER($1) AND is_active = TRUE
		LIMIT 1
	`, strings.TrimSpace(name)).Scan(&s.ID, &s.Name, &dbType, &s.Host, &s.Port, &s.Username, &authType, &sslMode, &s.IsActive, &s.CreatedAt, &s.UpdatedAt, &createdBy, &lastTest)
	if err != nil {
		return servers.Server{}, err
	}
	s.DBType = servers.DBType(dbType)
	s.AuthType = servers.AuthType(authType)
	s.SSLMode = servers.SSLMode(sslMode)
	s.CreatedBy = createdBy
	if lastTest.Valid {
		t := lastTest.Time.UTC()
		s.LastTestAt = &t
	}
	return s, nil
}

func (r *ServerRegistryRepository) GetEncrypted(ctx context.Context, id uuid.UUID) (servers.Server, []byte, []byte, error) {
	if r == nil || r.pool == nil {
		return servers.Server{}, nil, nil, fmt.Errorf("timescale not configured")
	}

	var s servers.Server
	var dbType, authType, sslMode string
	var encSecret, encDEK []byte
	var createdBy string
	var lastTest sql.NullTime
	err := r.pool.QueryRow(ctx, `
		SELECT  id, name, db_type, host, port, username, auth_type, encrypted_secret, encrypted_dek, ssl_mode, is_active, created_at, updated_at, COALESCE(created_by,''), last_test_at
		FROM optima_servers
		WHERE id = $1
	`, id).Scan(&s.ID, &s.Name, &dbType, &s.Host, &s.Port, &s.Username, &authType, &encSecret, &encDEK, &sslMode, &s.IsActive, &s.CreatedAt, &s.UpdatedAt, &createdBy, &lastTest)
	if err != nil {
		return servers.Server{}, nil, nil, err
	}
	s.DBType = servers.DBType(dbType)
	s.AuthType = servers.AuthType(authType)
	s.SSLMode = servers.SSLMode(sslMode)
	s.CreatedBy = createdBy
	if lastTest.Valid {
		t := lastTest.Time.UTC()
		s.LastTestAt = &t
	}
	return s, encSecret, encDEK, nil
}

func (r *ServerRegistryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	_, err := r.pool.Exec(ctx, "DELETE FROM optima_servers WHERE id = $1", id)
	return err
}

func (r *ServerRegistryRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	_, err := r.pool.Exec(ctx, "UPDATE optima_servers SET is_active = $1 WHERE id = $2", active, id)
	return err
}

func (r *ServerRegistryRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, name, host string, port int, username, sslMode string) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE optima_servers 
		SET name = $1, host = $2, port = $3, username = $4, ssl_mode = $5, updated_at = NOW()
		WHERE id = $6
	`, name, host, port, username, sslMode, id)
	return err
}

func (r *ServerRegistryRepository) UpdateCredentials(ctx context.Context, id uuid.UUID, encryptedSecret, encryptedDEK []byte) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE optima_servers 
		SET encrypted_secret = $1, encrypted_dek = $2, updated_at = NOW()
		WHERE id = $3
	`, encryptedSecret, encryptedDEK, id)
	return err
}

func (r *ServerRegistryRepository) TouchLastTest(ctx context.Context, id uuid.UUID, at time.Time) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("timescale not configured")
	}
	_, err := r.pool.Exec(ctx, "UPDATE optima_servers SET last_test_at = $1 WHERE id = $2", at, id)
	return err
}

func (r *ServerRegistryRepository) CheckDuplicate(ctx context.Context, excludeID uuid.UUID, name, host string, port int) (string, error) {
	if r == nil || r.pool == nil {
		return "", fmt.Errorf("timescale not configured")
	}
	var existingName string
	err := r.pool.QueryRow(ctx, `
		SELECT name FROM optima_servers 
		WHERE (UPPER(name) = UPPER($1) OR (UPPER(host) = UPPER($2) AND port = $3))
		AND id != $4
		LIMIT 1
	`, name, host, port, excludeID).Scan(&existingName)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return existingName, err
}
