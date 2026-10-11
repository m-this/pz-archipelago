package shell

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	// The fdbs are SQLite databases; this driver needs no C toolchain.
	_ "modernc.org/sqlite"
)

// editFDB opens a database's bytes in a scratch file, lets edit change it
// inside one transaction, and returns the bytes it leaves.
func editFDB(ctx context.Context, scratch string, db []byte, edit func(*sql.Tx) error) ([]byte, error) {
	path := filepath.Join(scratch, "edit.fdb")
	if err := os.WriteFile(path, db, 0o600); err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(path) }()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := inTransaction(ctx, conn, edit); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func inTransaction(ctx context.Context, conn *sql.DB, edit func(*sql.Tx) error) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := edit(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// execEach runs one statement once per argument list.
func execEach(ctx context.Context, tx *sql.Tx, query string, args [][]any) error {
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, a := range args {
		if _, err := stmt.ExecContext(ctx, a...); err != nil {
			return fmt.Errorf("%s %v: %w", query, a, err)
		}
	}
	return nil
}
