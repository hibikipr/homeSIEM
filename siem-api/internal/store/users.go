package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID          int64
	Subject     *string
	Email       *string
	DisplayName *string
	Role        string
	LocalHash   *string
	LastLoginAt *time.Time
	CreatedAt   time.Time
}

type RoleMapping struct {
	ID         int64
	GroupClaim string
	Role       string
	Priority   int
}

const userSelect = `SELECT id, subject, email, display_name, role, local_hash, last_login_at, created_at FROM users`

func scanUser(row rowScanner) (User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Subject, &u.Email, &u.DisplayName, &u.Role, &u.LocalHash,
		scanNullTime(&u.LastLoginAt), scanTime(&u.CreatedAt)); err != nil {
		return User{}, err
	}
	return u, nil
}

func (s *Store) UpsertUserBySubject(ctx context.Context, subject, email, displayName, role string) (User, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (subject, email, display_name, role)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(subject) DO UPDATE SET
			email = excluded.email,
			display_name = excluded.display_name,
			role = excluded.role
	`, subject, email, displayName, role)
	if err != nil {
		return User{}, err
	}

	row := s.db.QueryRowContext(ctx, userSelect+` WHERE subject = ?`, subject)
	return scanUser(row)
}

func (s *Store) TouchUserLogin(ctx context.Context, id int64, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, formatTime(at), id); err != nil {
		return err
	}

	uid := id
	target := "user:" + strconvItoa(id)
	if err := writeAudit(tx, AuditEntry{UserID: &uid, Action: "auth.login", Target: &target, Detail: "{}"}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EnsureLocalAdmin(ctx context.Context, username, passwordHash string) (User, error) {
	existing, err := s.GetLocalAdminByUsername(ctx, username)
	if err != nil {
		return User{}, err
	}
	if existing != nil {
		return *existing, nil
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO users (subject, display_name, role, local_hash) VALUES (NULL, ?, 'admin', ?)
	`, username, passwordHash)
	if err != nil {
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}

	row := s.db.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) GetLocalAdminByUsername(ctx context.Context, username string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		userSelect+` WHERE subject IS NULL AND display_name = ? AND local_hash IS NOT NULL`, username)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) ListRoleMappings(ctx context.Context) ([]RoleMapping, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, group_claim, role, priority FROM role_mappings ORDER BY priority ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RoleMapping
	for rows.Next() {
		var m RoleMapping
		if err := rows.Scan(&m.ID, &m.GroupClaim, &m.Role, &m.Priority); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) UpsertRoleMapping(ctx context.Context, m RoleMapping) (RoleMapping, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO role_mappings (group_claim, role, priority) VALUES (?, ?, ?)
		ON CONFLICT(group_claim) DO UPDATE SET role = excluded.role, priority = excluded.priority
	`, m.GroupClaim, m.Role, m.Priority)
	if err != nil {
		return RoleMapping{}, err
	}

	var out RoleMapping
	err = s.db.QueryRowContext(ctx,
		`SELECT id, group_claim, role, priority FROM role_mappings WHERE group_claim = ?`, m.GroupClaim).
		Scan(&out.ID, &out.GroupClaim, &out.Role, &out.Priority)
	return out, err
}

// EnsureBootstrapRoleMapping seeds a single admin role mapping, but only
// when the role_mappings table is completely empty - the fresh-database
// chicken-and-egg case this exists for (nobody can reach Settings to add a
// mapping without an admin role, and nobody has one yet). Checking "table
// empty" rather than "this group_claim doesn't already exist" is
// deliberate: once a real admin mapping exists, restarting the container
// must never re-seed or fight whatever they've configured since, even if
// they changed or removed the original bootstrap group (there's no delete
// endpoint yet, but priority/role edits alone should never be undone by a
// restart).
func (s *Store) EnsureBootstrapRoleMapping(ctx context.Context, groupClaim string) (RoleMapping, bool, error) {
	existing, err := s.ListRoleMappings(ctx)
	if err != nil {
		return RoleMapping{}, false, err
	}
	if len(existing) > 0 {
		return RoleMapping{}, false, nil
	}

	m, err := s.UpsertRoleMapping(ctx, RoleMapping{GroupClaim: groupClaim, Role: "admin", Priority: 1})
	if err != nil {
		return RoleMapping{}, false, err
	}
	return m, true, nil
}

func (s *Store) ResolveRole(ctx context.Context, groups []string) (string, bool) {
	mappings, err := s.ListRoleMappings(ctx)
	if err != nil {
		// Fails closed (deny) on any DB error, including transient SQLITE_BUSY
		// under the single-connection pool. Intentionally silent here: this
		// function's (string, bool) signature — matching auth.RoleResolver —
		// has no error channel, and adding a logger dependency to Store for
		// this one path isn't worth the ripple. A persistent DB problem will
		// surface elsewhere (every other Store method does log its errors).
		return "", false
	}

	memberOf := make(map[string]bool, len(groups))
	for _, g := range groups {
		memberOf[g] = true
	}

	for _, m := range mappings { // already ordered by priority ASC == "lowest wins"
		if memberOf[m.GroupClaim] {
			return m.Role, true
		}
	}
	return "", false
}
