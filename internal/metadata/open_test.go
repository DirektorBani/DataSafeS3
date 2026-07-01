package metadata

import "testing"

func TestPostgresDSNEncodesPassword(t *testing.T) {
	dsn := PostgresDSN(Config{
		PostgresUser:     "datasafe",
		PostgresPassword: "p@ss!word",
		PostgresHost:     "localhost",
		PostgresPort:     "5432",
		PostgresDB:       "datasafe",
	})
	want := "postgres://datasafe:p%40ss%21word@localhost:5432/datasafe?sslmode=disable"
	if dsn != want {
		t.Fatalf("PostgresDSN() = %q, want %q", dsn, want)
	}
}
