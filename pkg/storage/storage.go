package storage

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type Storage struct {
	db *sql.DB
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"-"`
}

func New(connStr string) (*Storage, error) {
	// Example connStr: "user=postgres password=postgres dbname=hydra sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	storage := &Storage{db: db}
	if err := storage.initDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	log.Println("Database initialized successfully")
	return storage, nil
}

func (s *Storage) initDB() error {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT UNIQUE,
		phone TEXT UNIQUE,
		password TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS contacts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		avatar TEXT,
		status TEXT
	);

	CREATE TABLE IF NOT EXISTS invites (
		token TEXT PRIMARY KEY,
		contact_info TEXT NOT NULL,
		expires_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sms_verifications (
		id SERIAL PRIMARY KEY,
		phone TEXT NOT NULL,
		code TEXT NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		verified BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS email_verifications (
		id SERIAL PRIMARY KEY,
		email TEXT NOT NULL,
		code TEXT NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		verified BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *Storage) CreateInvite(contactInfo string) (string, error) {
	token := fmt.Sprintf("invite-%d", time.Now().UnixNano())
	expiresAt := time.Now().Add(24 * time.Hour)

	query := "INSERT INTO invites (token, contact_info, expires_at) VALUES ($1, $2, $3)"
	_, err := s.db.Exec(query, token, contactInfo, expiresAt)
	if err != nil {
		return "", fmt.Errorf("failed to create invite: %w", err)
	}
	return token, nil
}

func (s *Storage) ValidateInvite(token string) (string, error) {
	var contactInfo string
	var expiresAt time.Time

	query := "SELECT contact_info, expires_at FROM invites WHERE token = $1"
	err := s.db.QueryRow(query, token).Scan(&contactInfo, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}

	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("token expired")
	}

	// Удаляем токен после использования
	deleteQuery := "DELETE FROM invites WHERE token = $1"
	_, err = s.db.Exec(deleteQuery, token)
	if err != nil {
		log.Printf("Failed to delete invite token: %v", err)
	}

	return contactInfo, nil
}

func (s *Storage) CreateUser(name, password, contactInfo string) (*User, error) {
	user := &User{
		ID:       fmt.Sprintf("user-%d", time.Now().UnixNano()),
		Name:     name,
		Password: password, // В реальном приложении пароль нужно хешировать
	}

	if strings.Contains(contactInfo, "@") {
		user.Email = contactInfo
	} else {
		user.Phone = contactInfo
	}

	query := "INSERT INTO users (id, name, email, phone, password) VALUES ($1, $2, $3, $4, $5)"
	_, err := s.db.Exec(query, user.ID, user.Name, user.Email, user.Phone, user.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// SMS Verification Methods
func (s *Storage) CreateSMSVerification(phone, code string) error {
	expiresAt := time.Now().Add(10 * time.Minute) // Код действителен 10 минут

	// Удаляем старые коды для этого номера
	_, err := s.db.Exec("DELETE FROM sms_verifications WHERE phone = $1", phone)
	if err != nil {
		return fmt.Errorf("failed to clean old codes: %w", err)
	}

	// Вставляем новый код
	query := "INSERT INTO sms_verifications (phone, code, expires_at) VALUES ($1, $2, $3)"
	_, err = s.db.Exec(query, phone, code, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to create SMS verification: %w", err)
	}

	return nil
}

func (s *Storage) ValidateSMSVerification(phone, code string) (bool, error) {
	var storedCode string
	var expiresAt time.Time

	query := "SELECT code, expires_at FROM sms_verifications WHERE phone = $1 AND verified = FALSE ORDER BY created_at DESC LIMIT 1"
	err := s.db.QueryRow(query, phone).Scan(&storedCode, &expiresAt)
	if err != nil {
		return false, fmt.Errorf("invalid or expired code: %w", err)
	}

	// Проверяем срок действия
	if time.Now().After(expiresAt) {
		return false, fmt.Errorf("code expired")
	}

	// Проверяем код
	if storedCode != code {
		return false, fmt.Errorf("invalid code")
	}

	// Помечаем код как использованный
	_, err = s.db.Exec("UPDATE sms_verifications SET verified = TRUE WHERE phone = $1 AND code = $2", phone, code)
	if err != nil {
		return false, fmt.Errorf("failed to mark code as verified: %w", err)
	}

	return true, nil
}

func (s *Storage) CreateEmailVerification(email, code string) error {
	expiresAt := time.Now().Add(10 * time.Minute)

	_, err := s.db.Exec("DELETE FROM email_verifications WHERE email = $1", email)
	if err != nil {
		return fmt.Errorf("failed to clean old codes: %w", err)
	}

	query := "INSERT INTO email_verifications (email, code, expires_at) VALUES ($1, $2, $3)"
	_, err = s.db.Exec(query, email, code, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to create email verification: %w", err)
	}

	return nil
}

func (s *Storage) ValidateEmailVerification(email, code string) (bool, error) {
	var storedCode string
	var expiresAt time.Time

	query := "SELECT code, expires_at FROM email_verifications WHERE email = $1 AND verified = FALSE ORDER BY created_at DESC LIMIT 1"
	err := s.db.QueryRow(query, email).Scan(&storedCode, &expiresAt)
	if err != nil {
		return false, fmt.Errorf("invalid or expired code: %w", err)
	}

	if time.Now().After(expiresAt) {
		return false, fmt.Errorf("code expired")
	}

	if storedCode != code {
		return false, fmt.Errorf("invalid code")
	}

	_, err = s.db.Exec("UPDATE email_verifications SET verified = TRUE WHERE email = $1 AND code = $2", email, code)
	if err != nil {
		return false, fmt.Errorf("failed to mark code as verified: %w", err)
	}

	return true, nil
}

func (s *Storage) GetUserByPhone(phone string) (*User, error) {
	user := &User{}
	query := "SELECT id, name, email, phone, password FROM users WHERE phone = $1"
	err := s.db.QueryRow(query, phone).Scan(&user.ID, &user.Name, &user.Email, &user.Phone, &user.Password)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return user, nil
}

func (s *Storage) GetUserByEmail(email string) (*User, error) {
	user := &User{}
	query := "SELECT id, name, email, phone, password FROM users WHERE email = $1"
	err := s.db.QueryRow(query, email).Scan(&user.ID, &user.Name, &user.Email, &user.Phone, &user.Password)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return user, nil
}

func (s *Storage) GetUser(id string) (*User, error) {
	user := &User{}
	query := "SELECT id, name, email, phone FROM users WHERE id = $1"
	err := s.db.QueryRow(query, id).Scan(&user.ID, &user.Name, &user.Email, &user.Phone)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

func (s *Storage) UpdateUser(user *User) error {
	query := "UPDATE users SET name = $1, email = $2, phone = $3 WHERE id = $4"
	_, err := s.db.Exec(query, user.Name, user.Email, user.Phone, user.ID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

func (s *Storage) DeleteUser(id string) error {
	query := "DELETE FROM users WHERE id = $1"
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

func (s *Storage) ValidateUser(contactInfo, password string) (*User, error) {
	user := &User{}
	var storedPassword string

	// Пытаемся найти пользователя по email или телефону
	query := "SELECT id, name, email, phone, password FROM users WHERE email = $1 OR phone = $2"
	err := s.db.QueryRow(query, contactInfo, contactInfo).Scan(&user.ID, &user.Name, &user.Email, &user.Phone, &storedPassword)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials: %w", err)
	}

	// В реальном приложении здесь должна быть проверка хеша
	if storedPassword != password {
		return nil, fmt.Errorf("invalid credentials")
	}

	return user, nil
}
