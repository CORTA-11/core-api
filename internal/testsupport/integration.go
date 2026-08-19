package testsupport

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
)

func RequiredEnv(t testing.TB, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required; run through make test-integration or make test-isolation", name)
	}
	return value
}

func OpenPostgres(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), RequiredEnv(t, "TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	return pool
}

func OpenRedis(t testing.TB) *redis.Client {
	t.Helper()
	options, err := redis.ParseURL(RequiredEnv(t, "TEST_REDIS_URL"))
	if err != nil {
		t.Fatalf("parse Redis URL: %v", err)
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("ping Redis: %v", err)
	}
	return client
}

func OpenMinIO(t testing.TB) *miniogo.Client {
	t.Helper()
	client, err := miniogo.New(RequiredEnv(t, "TEST_MINIO_ENDPOINT"), &miniogo.Options{
		Creds: credentials.NewStaticV4(RequiredEnv(t, "TEST_MINIO_ACCESS_KEY"), RequiredEnv(t, "TEST_MINIO_SECRET_KEY"), ""),
	})
	if err != nil {
		t.Fatalf("open MinIO: %v", err)
	}
	return client
}

func ApplyMigrations(t testing.TB, directory, databaseURL string) {
	t.Helper()
	m, err := migrate.New("file://"+filepath.Join(RepositoryRoot(), directory), databaseURL)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("apply migrations: %v", err)
	}
}

func RepositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("locate repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func DatabaseURLForSchema(t testing.TB, schema string) string {
	t.Helper()
	databaseURL, err := url.Parse(RequiredEnv(t, "TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	query := databaseURL.Query()
	query.Set("search_path", schema)
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}

func ResetPostgres(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	if err != nil {
		t.Fatalf("reset PostgreSQL: %v", err)
	}
}

func FlushRedis(t testing.TB, client *redis.Client) {
	t.Helper()
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("flush Redis: %v", err)
	}
}

func EmptyBucket(t testing.TB, client *miniogo.Client, bucket string) {
	t.Helper()
	ctx := context.Background()
	for object := range client.ListObjects(ctx, bucket, miniogo.ListObjectsOptions{Recursive: true}) {
		if object.Err != nil {
			t.Fatalf("list MinIO objects: %v", object.Err)
		}
		if err := client.RemoveObject(ctx, bucket, object.Key, miniogo.RemoveObjectOptions{}); err != nil {
			t.Fatalf("remove MinIO object: %v", err)
		}
	}
}

func CreateSchema(t testing.TB, pool *pgxpool.Pool, schema string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(`CREATE SCHEMA %q`, schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
}
