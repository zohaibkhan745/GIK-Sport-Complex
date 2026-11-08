package database

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"giki-sports-backend/internal/config"
)

// NewPool initializes and validates a PostgreSQL connection pool using pgxpool.
// It constructs the configuration using structured types rather than string concatenation,
// verifies initial connectivity within the configured timeout, and cleans up resources if startup fails.
func NewPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize pgxpool configuration: %w", sanitizeError(err, cfg.Password))
	}

	// Safely populate connection configuration fields directly without URL interpolation
	poolConfig.ConnConfig.Host = cfg.Host
	poolConfig.ConnConfig.Port = uint16(cfg.Port)
	poolConfig.ConnConfig.Database = cfg.Name
	poolConfig.ConnConfig.User = cfg.User
	poolConfig.ConnConfig.Password = cfg.Password

	// Explicit SSLMode configuration
	switch cfg.SSLMode {
	case "disable":
		poolConfig.ConnConfig.TLSConfig = nil
	case "require":
		// Require SSL/TLS without full CA certificate verification (standard for cloud staging)
		poolConfig.ConnConfig.TLSConfig = &tls.Config{
			ServerName:         cfg.Host,
			InsecureSkipVerify: true,
		}
	case "verify-ca", "verify-full":
		// Strict TLS verification
		poolConfig.ConnConfig.TLSConfig = &tls.Config{
			ServerName: cfg.Host,
		}
	default:
		return nil, fmt.Errorf("unsupported DB_SSL_MODE '%s' (allowed: disable, require, verify-ca, verify-full)", cfg.SSLMode)
	}

	// Apply pool constraints
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime

	// Bounded startup context
	timeout := cfg.ConnectTimeout
	if timeout <= 0 {
		timeout = config.DatabaseConfig{}.ConnectTimeout
	}
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool for host %s:%d: %w", cfg.Host, cfg.Port, sanitizeError(err, cfg.Password))
	}

	// Verify startup connectivity
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to verify initial connection to database %s on %s:%d: %w",
			cfg.Name, cfg.Host, cfg.Port, sanitizeError(err, cfg.Password))
	}

	return pool, nil
}

// sanitizeError replaces any occurrence of sensitive passwords from error strings.
func sanitizeError(err error, secret string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if secret != "" && strings.Contains(msg, secret) {
		msg = strings.ReplaceAll(msg, secret, "[REDACTED]")
		return fmt.Errorf("%s", msg)
	}
	return err
}

// NewSQLDB initializes and validates a *sql.DB connection using pgx/v5 stdlib driver.
// It constructs connection configuration directly without string interpolation or credential exposure.
func NewSQLDB(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
	connConfig, err := pgx.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize pgx connection configuration: %w", sanitizeError(err, cfg.Password))
	}

	connConfig.Host = cfg.Host
	connConfig.Port = uint16(cfg.Port)
	connConfig.Database = cfg.Name
	connConfig.User = cfg.User
	connConfig.Password = cfg.Password

	switch cfg.SSLMode {
	case "disable":
		connConfig.TLSConfig = nil
	case "require":
		connConfig.TLSConfig = &tls.Config{
			ServerName:         cfg.Host,
			InsecureSkipVerify: true,
		}
	case "verify-ca", "verify-full":
		connConfig.TLSConfig = &tls.Config{
			ServerName: cfg.Host,
		}
	default:
		return nil, fmt.Errorf("unsupported DB_SSL_MODE '%s' (allowed: disable, require, verify-ca, verify-full)", cfg.SSLMode)
	}

	db := stdlib.OpenDB(*connConfig)

	db.SetMaxOpenConns(int(cfg.MaxConns))
	db.SetMaxIdleConns(int(cfg.MinConns))
	db.SetConnMaxLifetime(cfg.MaxConnLifetime)
	db.SetConnMaxIdleTime(cfg.MaxConnIdleTime)

	timeout := cfg.ConnectTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to verify initial connection to database %s on %s:%d: %w",
			cfg.Name, cfg.Host, cfg.Port, sanitizeError(err, cfg.Password))
	}

	return db, nil
}

