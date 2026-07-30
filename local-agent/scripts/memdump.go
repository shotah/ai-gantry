//go:build ignore

package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	path := "gantry.db"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	ids := []any{}
	q := `SELECT id, kind, subject, content, created_at FROM memory WHERE superseded_by IS NULL`
	if len(os.Args) > 2 {
		q += ` AND id IN (` + placeholders(len(os.Args)-2) + `)`
		for _, a := range os.Args[2:] {
			ids = append(ids, a)
		}
		q += ` ORDER BY id`
	} else {
		q += ` ORDER BY id`
	}

	rows, err := db.Query(q, ids...)
	if err != nil {
		fatal(err)
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var id int
		var kind, subject, content, created string
		if err := rows.Scan(&id, &kind, &subject, &content, &created); err != nil {
			fatal(err)
		}
		n++
		fmt.Printf("--- id=%d kind=%s subject=%q created=%s\n%s\n\n", id, kind, subject, created, content)
	}
	if err := rows.Err(); err != nil {
		fatal(err)
	}
	if n == 0 {
		fmt.Println("(no matching rows)")
	}
}

func placeholders(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
