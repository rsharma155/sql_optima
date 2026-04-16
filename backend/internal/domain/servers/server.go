package servers

import (
	"errors"
	"strings"
	"time"
)

type DBType string

const (
	DBPostgres  DBType = "postgres"
	DBSQLServer DBType = "sqlserver"
)

type AuthType string

const (
	AuthStatic AuthType = "static"
)

type SSLMode string

type Server struct {
	ID         string
	Name       string
	DBType     DBType
	Host       string
	Port       int
	Username   string
	AuthType   AuthType
	SSLMode    SSLMode
	IsActive   bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	CreatedBy  string
	LastTestAt *time.Time
}

type CredentialPayload struct {
	Password                 string                 `json:"password"`
	SSLMode                  string                 `json:"sslmode,omitempty"`
	TrustServerCertificate   bool                   `json:"trust_server_certificate,omitempty"`
	Database                 string                 `json:"database,omitempty"` // PG: dbname (default postgres). SQL Server: initial catalog (default master).
	Extra                    map[string]interface{} `json:"extra,omitempty"`
}

type CreateServerInput struct {
	Name                   string
	DBType                 DBType
	Host                   string
	Port                   int
	Username               string
	Password               string
	SSLMode                string
	Database               string // optional; stored in encrypted credential payload
	TrustServerCertificate bool   // SQL Server: TrustServerCertificate in connection string
	Actor                  string
}

func (in CreateServerInput) Validate() error {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 128 {
		return errors.New("name must be 128 characters or fewer")
	}
	switch in.DBType {
	case DBPostgres, DBSQLServer:
	default:
		return errors.New("db_type must be 'postgres' or 'sqlserver'")
	}
	host := strings.TrimSpace(in.Host)
	if host == "" {
		return errors.New("host is required")
	}
	if len(host) > 253 {
		return errors.New("host must be 253 characters or fewer")
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return errors.New("host cannot contain whitespace")
	}
	if in.Port <= 0 || in.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	user := strings.TrimSpace(in.Username)
	if user == "" {
		return errors.New("username is required")
	}
	if len(user) > 128 {
		return errors.New("username must be 128 characters or fewer")
	}
	if in.Password == "" {
		return errors.New("password is required")
	}
	if len(in.Password) > 4096 {
		return errors.New("password must be 4096 characters or fewer")
	}
	db := strings.TrimSpace(in.Database)
	if len(db) > 128 {
		return errors.New("database/catalog name must be 128 characters or fewer")
	}
	for _, r := range db {
		if r < 0x20 || r == 0x7f {
			return errors.New("database/catalog name contains invalid characters")
		}
	}
	return nil
}
