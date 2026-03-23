package database

import (
	"encoding/json"

	"github.com/google/uuid"
)

type Plugin struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Author       string          `json:"author"`
	Version      string          `json:"version"`
	Namespace    string          `json:"namespace,omitempty"`
	Icon         string          `json:"icon,omitempty"`
	License      string          `json:"license,omitempty"`
	Homepage     string          `json:"homepage,omitempty"`
	MatchTypes   string          `json:"match_types"`    // comma-separated: "text,image" or "*"
	ConnectDomains string       `json:"connect_domains"` // comma-separated: "open.feishu.cn" or "*"
	GrantPerms   string          `json:"grant_perms"`     // comma-separated: "reply,skip" or "none"
	GithubURL    string          `json:"github_url"`
	CommitHash   string          `json:"commit_hash"`
	Script       string          `json:"script,omitempty"`
	ConfigSchema json.RawMessage `json:"config_schema"`
	Status       string          `json:"status"`
	RejectReason string          `json:"reject_reason,omitempty"`
	SubmittedBy  string          `json:"submitted_by"`
	ReviewedBy   string          `json:"reviewed_by,omitempty"`
	InstallCount int             `json:"install_count"`
	CreatedAt    int64           `json:"created_at"`
	UpdatedAt    int64           `json:"updated_at"`

	// Joined fields (not in DB, populated by queries)
	SubmitterName string `json:"submitter_name,omitempty"`
	ReviewerName  string `json:"reviewer_name,omitempty"`
}

// ConfigField describes a configurable parameter for a plugin.
type ConfigField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`        // "string", "string?"(optional), "number", "bool"
	Description string `json:"description"`
}

const pluginSelectCols = `p.id, p.name, p.description, p.author, p.version,
	p.namespace, p.icon, p.license, p.homepage,
	p.match_types, p.connect_domains, p.grant_perms,
	p.github_url, p.commit_hash, p.script, p.config_schema,
	p.status, p.reject_reason, p.submitted_by, p.reviewed_by, p.install_count,
	EXTRACT(EPOCH FROM p.created_at)::BIGINT, EXTRACT(EPOCH FROM p.updated_at)::BIGINT,
	COALESCE(su.username, ''), COALESCE(ru.username, '')`

const pluginFromJoin = ` FROM plugins p
	LEFT JOIN users su ON su.id = p.submitted_by
	LEFT JOIN users ru ON ru.id = p.reviewed_by`

func scanPlugin(scanner interface{ Scan(...any) error }) (*Plugin, error) {
	p := &Plugin{}
	err := scanner.Scan(&p.ID, &p.Name, &p.Description, &p.Author, &p.Version,
		&p.Namespace, &p.Icon, &p.License, &p.Homepage,
		&p.MatchTypes, &p.ConnectDomains, &p.GrantPerms,
		&p.GithubURL, &p.CommitHash, &p.Script, &p.ConfigSchema,
		&p.Status, &p.RejectReason, &p.SubmittedBy, &p.ReviewedBy, &p.InstallCount,
		&p.CreatedAt, &p.UpdatedAt,
		&p.SubmitterName, &p.ReviewerName)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (db *DB) CreatePlugin(p *Plugin) (*Plugin, error) {
	p.ID = uuid.New().String()
	if p.MatchTypes == "" {
		p.MatchTypes = "*"
	}
	if p.ConnectDomains == "" {
		p.ConnectDomains = "*"
	}
	_, err := db.Exec(`INSERT INTO plugins
		(id, name, description, author, version, namespace, icon, license, homepage,
		 match_types, connect_domains, grant_perms,
		 github_url, commit_hash, script, config_schema, status, submitted_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'pending',$17)`,
		p.ID, p.Name, p.Description, p.Author, p.Version,
		p.Namespace, p.Icon, p.License, p.Homepage,
		p.MatchTypes, p.ConnectDomains, p.GrantPerms,
		p.GithubURL, p.CommitHash, p.Script, p.ConfigSchema, p.SubmittedBy)
	if err != nil {
		return nil, err
	}
	p.Status = "pending"
	return p, nil
}

// FindPendingPlugin finds an existing pending plugin by submitter + name.
func (db *DB) FindPendingPlugin(submittedBy, name string) (*Plugin, error) {
	return scanPlugin(db.QueryRow("SELECT "+pluginSelectCols+pluginFromJoin+
		" WHERE p.submitted_by = $1 AND p.name = $2 AND p.status = 'pending'", submittedBy, name))
}

// UpdatePlugin updates an existing plugin's content (for resubmission).
func (db *DB) UpdatePlugin(id string, p *Plugin) error {
	_, err := db.Exec(`UPDATE plugins SET
		description=$1, author=$2, version=$3, namespace=$4, icon=$5, license=$6, homepage=$7,
		match_types=$8, connect_domains=$9, grant_perms=$10,
		github_url=$11, commit_hash=$12, script=$13, config_schema=$14,
		status='pending', reject_reason='', reviewed_by='', updated_at=NOW()
		WHERE id=$15`,
		p.Description, p.Author, p.Version, p.Namespace, p.Icon, p.License, p.Homepage,
		p.MatchTypes, p.ConnectDomains, p.GrantPerms,
		p.GithubURL, p.CommitHash, p.Script, p.ConfigSchema, id)
	return err
}

func (db *DB) GetPlugin(id string) (*Plugin, error) {
	return scanPlugin(db.QueryRow("SELECT "+pluginSelectCols+pluginFromJoin+" WHERE p.id = $1", id))
}

func (db *DB) ListPlugins(status string) ([]Plugin, error) {
	query := "SELECT " + pluginSelectCols + pluginFromJoin
	var args []any
	if status != "" {
		query += " WHERE p.status = $1"
		args = append(args, status)
	}
	query += " ORDER BY p.install_count DESC, p.created_at DESC"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plugins []Plugin
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, *p)
	}
	return plugins, rows.Err()
}

func (db *DB) UpdatePluginStatus(id, status, reviewedBy, reason string) error {
	_, err := db.Exec("UPDATE plugins SET status = $1, reviewed_by = $2, reject_reason = $3, updated_at = NOW() WHERE id = $4",
		status, reviewedBy, reason, id)
	return err
}

func (db *DB) RecordPluginInstall(pluginID, userID string) error {
	_, err := db.Exec(`INSERT INTO plugin_installs (plugin_id, user_id) VALUES ($1, $2)
		ON CONFLICT (plugin_id, user_id) DO UPDATE SET installed_at = NOW()`, pluginID, userID)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE plugins SET install_count = (SELECT COUNT(*) FROM plugin_installs WHERE plugin_id = $1) WHERE id = $1`, pluginID)
	return err
}

// ListPluginVersions returns all versions of a plugin by namespace/name.
func (db *DB) ListPluginVersions(namespace, name string) ([]Plugin, error) {
	rows, err := db.Query("SELECT "+pluginSelectCols+pluginFromJoin+
		" WHERE p.namespace = $1 AND p.name = $2 ORDER BY p.created_at DESC", namespace, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plugins []Plugin
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, *p)
	}
	return plugins, rows.Err()
}

func (db *DB) DeletePlugin(id string) error {
	db.Exec("DELETE FROM plugin_installs WHERE plugin_id = $1", id)
	_, err := db.Exec("DELETE FROM plugins WHERE id = $1", id)
	return err
}
