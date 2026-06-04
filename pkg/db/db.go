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
	ID   int    `json:"id"`
	Item string `json:"item"`
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
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
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

func (d *DB) AddTodoItem(email, item string) error {
	query := `INSERT INTO todo_items (email, item) VALUES ($1, $2)`
	_, err := d.sqlDB.Exec(query, email, item)
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

func (d *DB) GetTodoItems(email string) ([]TodoItem, error) {
	query := `SELECT id, item FROM todo_items WHERE email = $1 ORDER BY id ASC`
	rows, err := d.sqlDB.Query(query, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TodoItem
	for rows.Next() {
		var item TodoItem
		if err := rows.Scan(&item.ID, &item.Item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
