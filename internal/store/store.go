package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/fndns/manager/internal/id"
	"github.com/fndns/manager/internal/model"
)

var ErrNotFound = errors.New("未找到数据")

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	schema := `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS credentials (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  secret_hint TEXT NOT NULL DEFAULT '',
  encrypted_secret BLOB NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_sync_at TEXT,
  last_sync_status TEXT NOT NULL DEFAULT 'never',
  last_sync_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS domains (
  id TEXT PRIMARY KEY,
  credential_id TEXT NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
  remote_id TEXT NOT NULL,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT '',
  grade TEXT NOT NULL DEFAULT '',
  record_count INTEGER NOT NULL DEFAULT 0,
  last_sync_at TEXT,
  sync_error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(credential_id, remote_id)
);

CREATE TABLE IF NOT EXISTS records (
  id TEXT PRIMARY KEY,
  domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
  remote_id TEXT NOT NULL,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  value TEXT NOT NULL,
  ttl INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT '',
  line TEXT NOT NULL DEFAULT '',
  line_id TEXT NOT NULL DEFAULT '',
  mx INTEGER NOT NULL DEFAULT 0,
  weight INTEGER NOT NULL DEFAULT 0,
  proxied INTEGER NOT NULL DEFAULT 0,
  supports_proxied INTEGER NOT NULL DEFAULT 0,
  supports_disable INTEGER NOT NULL DEFAULT 0,
  remark TEXT NOT NULL DEFAULT '',
  remote_updated_at TEXT,
  last_sync_at TEXT NOT NULL,
  UNIQUE(domain_id, remote_id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  operator TEXT NOT NULL,
  credential_id TEXT NOT NULL DEFAULT '',
  credential_name TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  domain TEXT NOT NULL DEFAULT '',
  record_name TEXT NOT NULL DEFAULT '',
  record_type TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  result TEXT NOT NULL,
  message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_domains_credential ON domains(credential_id);
CREATE INDEX IF NOT EXISTS idx_domains_name ON domains(name);
CREATE INDEX IF NOT EXISTS idx_records_domain ON records(domain_id);
CREATE INDEX IF NOT EXISTS idx_logs_created ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_logs_filters ON audit_logs(credential_id, domain, action, result);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("初始化数据库: %w", err)
	}
	return nil
}

func timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	return t
}

func optionalTime(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	t := parseTime(v.String)
	return &t
}

func (s *Store) CreateCredential(ctx context.Context, credential model.StoredCredential) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO credentials
    (id, name, provider, secret_hint, encrypted_secret, created_at, updated_at, last_sync_status)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, credential.ID, credential.Name, credential.Provider,
		credential.SecretHint, credential.EncryptedSecret, timestamp(credential.CreatedAt),
		timestamp(credential.UpdatedAt), model.SyncNever)
	if err != nil {
		return fmt.Errorf("保存凭据: %w", err)
	}
	return nil
}

func (s *Store) UpdateCredential(ctx context.Context, credential model.StoredCredential) error {
	result, err := s.db.ExecContext(ctx, `UPDATE credentials SET name=?, provider=?, secret_hint=?,
    encrypted_secret=?, updated_at=? WHERE id=?`, credential.Name, credential.Provider,
		credential.SecretHint, credential.EncryptedSecret, timestamp(credential.UpdatedAt), credential.ID)
	if err != nil {
		return fmt.Errorf("更新凭据: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetCredential(ctx context.Context, credentialID string) (model.StoredCredential, error) {
	var item model.StoredCredential
	var createdAt, updatedAt string
	var lastSync sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, name, provider, secret_hint, encrypted_secret,
    created_at, updated_at, last_sync_at, last_sync_status, last_sync_error
    FROM credentials WHERE id=?`, credentialID).Scan(&item.ID, &item.Name, &item.Provider,
		&item.SecretHint, &item.EncryptedSecret, &createdAt, &updatedAt, &lastSync,
		&item.LastSyncStatus, &item.LastSyncError)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, fmt.Errorf("读取凭据: %w", err)
	}
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	item.LastSyncAt = optionalTime(lastSync)
	return item, nil
}

func (s *Store) ListCredentials(ctx context.Context) ([]model.Credential, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.name, c.provider, c.secret_hint, c.created_at,
    c.updated_at, c.last_sync_at, c.last_sync_status, c.last_sync_error, COUNT(d.id)
    FROM credentials c LEFT JOIN domains d ON d.credential_id=c.id
    GROUP BY c.id ORDER BY c.created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("读取凭据列表: %w", err)
	}
	defer rows.Close()
	items := make([]model.Credential, 0)
	for rows.Next() {
		var item model.Credential
		var createdAt, updatedAt string
		var lastSync sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &item.Provider, &item.SecretHint, &createdAt,
			&updatedAt, &lastSync, &item.LastSyncStatus, &item.LastSyncError, &item.DomainCount); err != nil {
			return nil, fmt.Errorf("解析凭据列表: %w", err)
		}
		item.CreatedAt = parseTime(createdAt)
		item.UpdatedAt = parseTime(updatedAt)
		item.LastSyncAt = optionalTime(lastSync)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DeleteCredential(ctx context.Context, credentialID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id=?`, credentialID)
	if err != nil {
		return fmt.Errorf("删除凭据: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetCredentialSync(ctx context.Context, credentialID, status, message string, synced bool) error {
	var result sql.Result
	var err error
	if synced {
		result, err = s.db.ExecContext(ctx, `UPDATE credentials SET last_sync_at=?, last_sync_status=?, last_sync_error=? WHERE id=?`,
			timestamp(s.now()), status, message, credentialID)
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE credentials SET last_sync_status=?, last_sync_error=? WHERE id=?`, status, message, credentialID)
	}
	if err != nil {
		return fmt.Errorf("更新同步状态: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SyncDomains(ctx context.Context, credentialID string, remote []model.RemoteDomain) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existingRows, err := tx.QueryContext(ctx, `SELECT remote_id, id FROM domains WHERE credential_id=?`, credentialID)
	if err != nil {
		return err
	}
	existing := map[string]string{}
	for existingRows.Next() {
		var remoteID, domainID string
		if err := existingRows.Scan(&remoteID, &domainID); err != nil {
			existingRows.Close()
			return err
		}
		existing[remoteID] = domainID
	}
	existingRows.Close()
	now := timestamp(s.now())
	seen := map[string]bool{}
	for _, domain := range remote {
		seen[domain.RemoteID] = true
		domainID := existing[domain.RemoteID]
		if domainID == "" {
			domainID = id.New()
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO domains
      (id, credential_id, remote_id, name, status, grade, record_count, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(credential_id, remote_id) DO UPDATE SET
        name=excluded.name, status=excluded.status, grade=excluded.grade,
        record_count=excluded.record_count, updated_at=excluded.updated_at`, domainID,
			credentialID, domain.RemoteID, strings.ToLower(domain.Name), domain.Status, domain.Grade,
			domain.RecordCount, now, now)
		if err != nil {
			return fmt.Errorf("保存域名 %s: %w", domain.Name, err)
		}
	}
	for remoteID := range existing {
		if !seen[remoteID] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM domains WHERE credential_id=? AND remote_id=?`, credentialID, remoteID); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交域名同步: %w", err)
	}
	return nil
}

func (s *Store) scanDomain(scanner interface{ Scan(...any) error }) (model.Domain, error) {
	var item model.Domain
	var lastSync sql.NullString
	var createdAt, updatedAt string
	err := scanner.Scan(&item.ID, &item.CredentialID, &item.CredentialName, &item.Provider,
		&item.RemoteID, &item.Name, &item.Status, &item.Grade, &item.RecordCount,
		&lastSync, &item.SyncError, &createdAt, &updatedAt)
	if err != nil {
		return item, err
	}
	item.LastSyncAt = optionalTime(lastSync)
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	item.CacheState = model.CacheNever
	if item.LastSyncAt != nil {
		item.CacheState = model.CacheFresh
		if item.LastSyncAt.Before(s.now().AddDate(0, -6, 0)) {
			item.CacheState = model.CacheExpired
		}
	}
	return item, nil
}

const domainSelect = `SELECT d.id, d.credential_id, c.name, c.provider, d.remote_id, d.name,
  d.status, d.grade, d.record_count, d.last_sync_at, d.sync_error, d.created_at, d.updated_at
  FROM domains d JOIN credentials c ON c.id=d.credential_id`

func (s *Store) ListDomains(ctx context.Context) ([]model.Domain, error) {
	rows, err := s.db.QueryContext(ctx, domainSelect+` ORDER BY c.created_at ASC, c.id ASC, d.name ASC`)
	if err != nil {
		return nil, fmt.Errorf("读取域名列表: %w", err)
	}
	defer rows.Close()
	items := make([]model.Domain, 0)
	seen := map[string]bool{}
	for rows.Next() {
		item, err := s.scanDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("解析域名列表: %w", err)
		}
		key := strings.ToLower(strings.TrimSuffix(item.Name, "."))
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, rows.Err()
}

func (s *Store) GetDomain(ctx context.Context, domainID string) (model.Domain, error) {
	item, err := s.scanDomain(s.db.QueryRowContext(ctx, domainSelect+` WHERE d.id=?`, domainID))
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, fmt.Errorf("读取域名: %w", err)
	}
	return item, nil
}

func (s *Store) SetDomainSyncError(ctx context.Context, domainID, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE domains SET sync_error=? WHERE id=?`, message, domainID)
	return err
}

func (s *Store) SyncRecords(ctx context.Context, domainID string, remote []model.RemoteRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existingRows, err := tx.QueryContext(ctx, `SELECT remote_id, id FROM records WHERE domain_id=?`, domainID)
	if err != nil {
		return err
	}
	existing := map[string]string{}
	for existingRows.Next() {
		var remoteID, recordID string
		if err := existingRows.Scan(&remoteID, &recordID); err != nil {
			existingRows.Close()
			return err
		}
		existing[remoteID] = recordID
	}
	existingRows.Close()
	now := s.now()
	seen := map[string]bool{}
	for _, record := range remote {
		seen[record.RemoteID] = true
		var updatedAt any
		if record.UpdatedAt != nil {
			updatedAt = timestamp(*record.UpdatedAt)
		}
		recordID := existing[record.RemoteID]
		if recordID == "" {
			recordID = id.New()
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO records
      (id, domain_id, remote_id, name, type, value, ttl, status, line, line_id, mx,
       weight, proxied, supports_proxied, supports_disable, remark, remote_updated_at, last_sync_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(domain_id, remote_id) DO UPDATE SET
        name=excluded.name, type=excluded.type, value=excluded.value, ttl=excluded.ttl,
        status=excluded.status, line=excluded.line, line_id=excluded.line_id, mx=excluded.mx,
        weight=excluded.weight, proxied=excluded.proxied,
        supports_proxied=excluded.supports_proxied, supports_disable=excluded.supports_disable,
        remark=excluded.remark, remote_updated_at=excluded.remote_updated_at,
        last_sync_at=excluded.last_sync_at`, recordID, domainID,
			record.RemoteID, record.Name, strings.ToUpper(record.Type), record.Value, record.TTL,
			record.Status, record.Line, record.LineID, record.MX, record.Weight, record.Proxied,
			record.SupportsProxied, record.SupportsDisable, record.Remark, updatedAt, timestamp(now))
		if err != nil {
			return fmt.Errorf("保存记录 %s: %w", record.Name, err)
		}
	}
	for remoteID := range existing {
		if !seen[remoteID] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM records WHERE domain_id=? AND remote_id=?`, domainID, remoteID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE domains SET record_count=?, last_sync_at=?, sync_error='', updated_at=? WHERE id=?`,
		len(remote), timestamp(now), timestamp(now), domainID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交记录同步: %w", err)
	}
	return nil
}

func scanRecord(scanner interface{ Scan(...any) error }) (model.Record, error) {
	var item model.Record
	var proxied, supportsProxied, supportsDisable int
	var updatedAt sql.NullString
	var lastSync string
	err := scanner.Scan(&item.ID, &item.DomainID, &item.RemoteID, &item.Name, &item.Type,
		&item.Value, &item.TTL, &item.Status, &item.Line, &item.LineID, &item.MX,
		&item.Weight, &proxied, &supportsProxied, &supportsDisable, &item.Remark,
		&updatedAt, &lastSync)
	if err != nil {
		return item, err
	}
	item.Proxied = proxied != 0
	item.SupportsProxied = supportsProxied != 0
	item.SupportsDisable = supportsDisable != 0
	item.RemoteUpdatedAt = optionalTime(updatedAt)
	item.LastSyncAt = parseTime(lastSync)
	return item, nil
}

const recordSelect = `SELECT id, domain_id, remote_id, name, type, value, ttl, status,
  line, line_id, mx, weight, proxied, supports_proxied, supports_disable, remark,
  remote_updated_at, last_sync_at FROM records`

func (s *Store) ListRecords(ctx context.Context, domainID string) ([]model.Record, error) {
	rows, err := s.db.QueryContext(ctx, recordSelect+` WHERE domain_id=? ORDER BY name ASC, type ASC`, domainID)
	if err != nil {
		return nil, fmt.Errorf("读取解析记录: %w", err)
	}
	defer rows.Close()
	items := make([]model.Record, 0)
	for rows.Next() {
		item, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetRecord(ctx context.Context, domainID, recordID string) (model.Record, error) {
	item, err := scanRecord(s.db.QueryRowContext(ctx, recordSelect+` WHERE domain_id=? AND id=?`, domainID, recordID))
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, fmt.Errorf("读取解析记录: %w", err)
	}
	return item, nil
}

func (s *Store) AppendLog(ctx context.Context, log model.AuditLog) error {
	if log.ID == "" {
		log.ID = id.New()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = s.now()
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM audit_logs WHERE created_at < ?`, timestamp(s.now().AddDate(0, -6, 0))); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_logs
    (id, created_at, operator, credential_id, credential_name, provider, domain,
     record_name, record_type, action, result, message)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, log.ID, timestamp(log.CreatedAt), log.Operator,
		log.CredentialID, log.CredentialName, log.Provider, log.Domain, log.RecordName,
		log.RecordType, log.Action, log.Result, log.Message)
	return err
}

func (s *Store) ListLogs(ctx context.Context, filter model.LogFilter) ([]model.AuditLog, error) {
	where := []string{"1=1"}
	args := make([]any, 0)
	if filter.From != nil {
		where = append(where, "created_at >= ?")
		args = append(args, timestamp(*filter.From))
	}
	if filter.To != nil {
		where = append(where, "created_at <= ?")
		args = append(args, timestamp(*filter.To))
	}
	for _, field := range []struct{ column, value string }{
		{"credential_id", filter.CredentialID}, {"domain", filter.Domain},
		{"action", filter.Action}, {"result", filter.Result},
	} {
		if field.value != "" {
			where = append(where, field.column+" = ?")
			args = append(args, field.value)
		}
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit, max(filter.Offset, 0))
	query := `SELECT id, created_at, operator, credential_id, credential_name, provider,
    domain, record_name, record_type, action, result, message FROM audit_logs WHERE ` +
		strings.Join(where, " AND ") + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("读取操作日志: %w", err)
	}
	defer rows.Close()
	items := make([]model.AuditLog, 0)
	for rows.Next() {
		var item model.AuditLog
		var createdAt string
		if err := rows.Scan(&item.ID, &createdAt, &item.Operator, &item.CredentialID,
			&item.CredentialName, &item.Provider, &item.Domain, &item.RecordName,
			&item.RecordType, &item.Action, &item.Result, &item.Message); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CleanupLogs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM audit_logs WHERE created_at < ?`, timestamp(s.now().AddDate(0, -6, 0)))
	return err
}

func (s *Store) Overview(ctx context.Context) (model.Overview, error) {
	var overview model.Overview
	credentials, err := s.ListCredentials(ctx)
	if err != nil {
		return overview, err
	}
	overview.CredentialCount = len(credentials)
	for _, credential := range credentials {
		if credential.LastSyncStatus == model.SyncError {
			overview.CredentialFailures++
		}
		if credential.LastSyncAt != nil && (overview.LastSyncAt == nil || credential.LastSyncAt.After(*overview.LastSyncAt)) {
			t := *credential.LastSyncAt
			overview.LastSyncAt = &t
		}
	}
	domains, err := s.ListDomains(ctx)
	if err != nil {
		return overview, err
	}
	overview.DomainCount = len(domains)
	for _, domain := range domains {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM records WHERE domain_id=?`, domain.ID).Scan(&count); err != nil {
			return overview, err
		}
		overview.RecordCount += count
	}
	return overview, nil
}
