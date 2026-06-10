package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

type Mail struct {
	ID        int       `json:"id"`
	From      string    `json:"from_email"`
	To        string    `json:"to_email"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	IsRead    bool      `json:"is_read"`
	Timestamp time.Time `json:"timestamp"`
}

type MailSummary struct {
	ID        int       `json:"id"`
	From      string    `json:"from_email"`
	Subject   string    `json:"subject"`
	Timestamp time.Time `json:"timestamp"`
	IsRead    bool      `json:"is_read"`
}

type TodoItem struct {
	ID          int    `json:"id"`
	Item        string `json:"item"`
	Details     string `json:"details"`
	TaskBlocked bool   `json:"task_blocked"`
}

type DB struct {
	connStr string
	sqlDB   *sql.DB
}

func NewDB(connStr string) (*DB, error) {
	sqlDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}
	return &DB{connStr: connStr, sqlDB: sqlDB}, nil
}

func (d *DB) Close() error {
	return d.sqlDB.Close()
}

func (d *DB) SetupSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS emails (
		id SERIAL PRIMARY KEY,
		from_email TEXT NOT NULL,
		to_email TEXT NOT NULL,
		subject TEXT NOT NULL,
		body TEXT NOT NULL,
		is_read BOOLEAN DEFAULT FALSE,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agent_logs (
		id SERIAL PRIMARY KEY,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		agent_email TEXT NOT NULL,
		action TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS todo_items (
		id SERIAL PRIMARY KEY,
		email TEXT NOT NULL,
		item TEXT NOT NULL,
		details TEXT DEFAULT '',
		task_blocked BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	ALTER TABLE todo_items ADD COLUMN IF NOT EXISTS details TEXT DEFAULT '';
	ALTER TABLE todo_items ADD COLUMN IF NOT EXISTS task_blocked BOOLEAN DEFAULT FALSE;

	CREATE TABLE IF NOT EXISTS agent_history (
		agent_email TEXT PRIMARY KEY,
		history JSONB NOT NULL,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := d.sqlDB.Exec(query)
	return err
}

func (d *DB) ClearTables() error {
	if err := d.SetupSchema(); err != nil {
		return err
	}
	query := `
	TRUNCATE TABLE emails, agent_logs, todo_items, agent_history RESTART IDENTITY CASCADE;
	`
	_, err := d.sqlDB.Exec(query)
	return err
}

func (d *DB) LogAction(agentEmail, action string) error {
	query := `INSERT INTO agent_logs (agent_email, action) VALUES ($1, $2) RETURNING id`
	var id int
	err := d.sqlDB.QueryRow(query, agentEmail, action).Scan(&id)
	if err != nil {
		return err
	}

	payload := fmt.Sprintf("%d", id)
	notifyQuery := fmt.Sprintf(`NOTIFY log_events, %s;`, pq.QuoteLiteral(payload))
	_, err = d.sqlDB.Exec(notifyQuery)
	return err
}

func (d *DB) GetLog(id int) (time.Time, string, string, error) {
	var t time.Time
	var email, action string
	query := `SELECT timestamp, agent_email, action FROM agent_logs WHERE id = $1`
	err := d.sqlDB.QueryRow(query, id).Scan(&t, &email, &action)
	return t, email, action, err
}

func (d *DB) SendMail(from, to, subject, body string) error {
	query := `INSERT INTO emails (from_email, to_email, subject, body) VALUES ($1, $2, $3, $4)`
	_, err := d.sqlDB.Exec(query, from, to, subject, body)
	if err != nil {
		return err
	}

	// Notify the recipient
	notifyQuery := fmt.Sprintf(`NOTIFY mail_events, '%s';`, to)
	_, err = d.sqlDB.Exec(notifyQuery)
	return err
}

func (d *DB) GetUnreadSummary(email string) ([]MailSummary, error) {
	query := `SELECT id, from_email, subject, timestamp, is_read FROM emails WHERE to_email = $1 AND is_read = FALSE ORDER BY timestamp ASC`
	rows, err := d.sqlDB.Query(query, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []MailSummary
	for rows.Next() {
		var s MailSummary
		if err := rows.Scan(&s.ID, &s.From, &s.Subject, &s.Timestamp, &s.IsRead); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

func (d *DB) GetAllSummary(email string) ([]MailSummary, error) {
	query := `SELECT id, from_email, subject, timestamp, is_read FROM emails WHERE to_email = $1 ORDER BY timestamp DESC`
	rows, err := d.sqlDB.Query(query, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []MailSummary
	for rows.Next() {
		var s MailSummary
		if err := rows.Scan(&s.ID, &s.From, &s.Subject, &s.Timestamp, &s.IsRead); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

func (d *DB) ReadMail(email string, id int) (*Mail, error) {
	// First mark as read
	updateQuery := `UPDATE emails SET is_read = TRUE WHERE id = $1 AND to_email = $2`
	_, err := d.sqlDB.Exec(updateQuery, id, email)
	if err != nil {
		return nil, err
	}

	// Fetch full mail
	query := `SELECT id, from_email, to_email, subject, body, is_read, timestamp FROM emails WHERE id = $1 AND to_email = $2`
	row := d.sqlDB.QueryRow(query, id, email)

	var m Mail
	err = row.Scan(&m.ID, &m.From, &m.To, &m.Subject, &m.Body, &m.IsRead, &m.Timestamp)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *DB) WaitForMail(email string) error {
	// First check if there is unread mail immediately
	summaries, err := d.GetUnreadSummary(email)
	if err != nil {
		return err
	}
	if len(summaries) > 0 {
		return nil // Mail is already waiting
	}

	// Setup listener
	reportProblem := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("Listener error: %v", err)
		}
	}

	listener := pq.NewListener(d.connStr, 10*time.Second, time.Minute, reportProblem)
	defer listener.Close()

	err = listener.Listen("mail_events")
	if err != nil {
		return err
	}

	for {
		select {
		case n := <-listener.Notify:
			if n.Extra == email {
				return nil // We got mail!
			}
		case <-time.After(30 * time.Second):
			// periodically wake up and re-check DB just in case we missed a notification
			// or ping the connection to keep it alive
			go listener.Ping()
			summaries, _ := d.GetUnreadSummary(email)
			if len(summaries) > 0 {
				return nil
			}
		}
	}
}

func (d *DB) AddTodoItem(email, item, details string, taskBlocked bool) error {
	query := `INSERT INTO todo_items (email, item, details, task_blocked) VALUES ($1, $2, $3, $4)`
	_, err := d.sqlDB.Exec(query, email, item, details, taskBlocked)
	return err
}

func (d *DB) RemoveTodoItem(email string, id int) error {
	query := `DELETE FROM todo_items WHERE id = $1 AND email = $2`
	res, err := d.sqlDB.Exec(query, id, email)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("todo item ID %d not found or not owned by you", id)
	}
	return nil
}

func (d *DB) GetTodoItems(email string, excludeBlocked bool) ([]TodoItem, error) {
	query := `SELECT id, item, task_blocked FROM todo_items WHERE email = $1`
	if excludeBlocked {
		query += ` AND task_blocked = FALSE`
	}
	query += ` ORDER BY id ASC`
	rows, err := d.sqlDB.Query(query, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TodoItem
	for rows.Next() {
		var item TodoItem
		if err := rows.Scan(&item.ID, &item.Item, &item.TaskBlocked); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (d *DB) GetTodoItem(email string, id int) (*TodoItem, error) {
	query := `SELECT id, item, details, task_blocked FROM todo_items WHERE id = $1 AND email = $2`
	row := d.sqlDB.QueryRow(query, id, email)
	var item TodoItem
	err := row.Scan(&item.ID, &item.Item, &item.Details, &item.TaskBlocked)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *DB) UpdateTodoBlockedState(email string, id int, blocked bool) error {
	query := `UPDATE todo_items SET task_blocked = $1 WHERE id = $2 AND email = $3`
	res, err := d.sqlDB.Exec(query, blocked, id, email)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("todo item ID %d not found or not owned by you", id)
	}
	return nil
}

type TodoItemWithOwner struct {
	TodoItem
	Email string `json:"email"`
}

func (d *DB) ListAllTodos(emailFilter string) ([]TodoItemWithOwner, error) {
	query := `SELECT id, email, item, details, task_blocked FROM todo_items`
	var args []interface{}
	if emailFilter != "" {
		query += ` WHERE email = $1`
		args = append(args, emailFilter)
	}
	query += ` ORDER BY email ASC, id ASC`
	rows, err := d.sqlDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TodoItemWithOwner
	for rows.Next() {
		var item TodoItemWithOwner
		if err := rows.Scan(&item.ID, &item.Email, &item.Item, &item.Details, &item.TaskBlocked); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

type EmailDetails struct {
	ID        int       `json:"id"`
	From      string    `json:"from_email"`
	To        string    `json:"to_email"`
	Subject   string    `json:"subject"`
	IsRead    bool      `json:"is_read"`
	Timestamp time.Time `json:"timestamp"`
}

func (d *DB) ListAllEmails(accountFilter string, onlyUnread bool, sinceFilter time.Time) ([]EmailDetails, error) {
	query := `SELECT id, from_email, to_email, subject, is_read, timestamp FROM emails WHERE 1=1`
	var args []interface{}
	placeholderIdx := 1

	if accountFilter != "" {
		query += fmt.Sprintf(` AND (to_email = $%d OR from_email = $%d)`, placeholderIdx, placeholderIdx)
		args = append(args, accountFilter)
		placeholderIdx++
	}

	if onlyUnread {
		query += ` AND is_read = FALSE`
	}

	if !sinceFilter.IsZero() {
		query += fmt.Sprintf(` AND timestamp >= $%d`, placeholderIdx)
		args = append(args, sinceFilter)
		placeholderIdx++
	}

	query += ` ORDER BY timestamp DESC`

	rows, err := d.sqlDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []EmailDetails
	for rows.Next() {
		var email EmailDetails
		if err := rows.Scan(&email.ID, &email.From, &email.To, &email.Subject, &email.IsRead, &email.Timestamp); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, nil
}

func (d *DB) GetEmailByID(id int) (*Mail, error) {
	query := `SELECT id, from_email, to_email, subject, body, is_read, timestamp FROM emails WHERE id = $1`
	row := d.sqlDB.QueryRow(query, id)

	var m Mail
	err := row.Scan(&m.ID, &m.From, &m.To, &m.Subject, &m.Body, &m.IsRead, &m.Timestamp)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *DB) SaveAgentHistory(agentEmail string, historyJSON []byte) error {
	query := `
	INSERT INTO agent_history (agent_email, history, updated_at)
	VALUES ($1, $2, CURRENT_TIMESTAMP)
	ON CONFLICT (agent_email)
	DO UPDATE SET history = $2, updated_at = CURRENT_TIMESTAMP
	`
	_, err := d.sqlDB.Exec(query, agentEmail, historyJSON)
	return err
}

func (d *DB) LoadAgentHistory(agentEmail string) ([]byte, error) {
	query := `SELECT history FROM agent_history WHERE agent_email = $1`
	var historyJSON []byte
	err := d.sqlDB.QueryRow(query, agentEmail).Scan(&historyJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return historyJSON, err
}
