module github.com/rsharma155/sql_optima

go 1.25.7

require (
	github.com/expr-lang/expr v1.17.8
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/gorilla/mux v1.8.1
	github.com/jackc/pgx/v5 v5.9.1
	github.com/lib/pq v1.10.9
	github.com/microsoft/go-mssqldb v1.9.8
	github.com/parquet-go/parquet-go v0.29.0
	github.com/yourorg/pg_explain_analyze v0.0.0
	golang.org/x/crypto v0.48.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/yourorg/pg_explain_analyze => ../../pg_explain_analyze

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/golang-sql/civil v0.0.0-20220223132316-b832511892a9 // indirect
	github.com/golang-sql/sqlexp v0.1.0 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/parquet-go/bitpack v1.0.0 // indirect
	github.com/parquet-go/jsonlite v1.0.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/twpayne/go-geom v1.6.1 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/protobuf v1.36.1 // indirect
)
