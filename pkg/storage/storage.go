package storage

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
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

func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite3", dbPath)
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
		expires_at DATETIME NOT NULL
	);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *Storage) CreateInvite(contactInfo string) (string, error) {
	token := fmt.Sprintf("invite-%d", time.Now().UnixNano())
	expiresAt := time.Now().Add(24 * time.Hour)

	query := "INSERT INTO invites (token, contact_info, expires_at) VALUES (?, ?, ?)"
	_, err := s.db.Exec(query, token, contactInfo, expiresAt)
	if err != nil {
		return "", fmt.Errorf("failed to create invite: %w", err)
	}
	return token, nil
}

func (s *Storage) ValidateInvite(token string) (string, error) {
	var contactInfo string
	var expiresAt time.Time

	query := "SELECT contact_info, expires_at FROM invites WHERE token = ?"
	err := s.db.QueryRow(query, token).Scan(&contactInfo, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}

	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("token expired")
	}

	// Удаляем токен после использования
	deleteQuery := "DELETE FROM invites WHERE token = ?"
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

	query := "INSERT INTO users (id, name, email, phone, password) VALUES (?, ?, ?, ?, ?)"
	_, err := s.db.Exec(query, user.ID, user.Name, user.Email, user.Phone, user.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

func (s *Storage) GetUser(id string) (*User, error) {
	user := &User{}
	query := "SELECT id, name, email, phone FROM users WHERE id = ?"
	err := s.db.QueryRow(query, id).Scan(&user.ID, &user.Name, &user.Email, &user.Phone)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return user, nil
}

func (s *Storage) UpdateUser(user *User) error {
	query := "UPDATE users SET name = ?, email = ?, phone = ? WHERE id = ?"
	_, err := s.db.Exec(query, user.Name, user.Email, user.Phone, user.ID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

func (s *Storage) DeleteUser(id string) error {
	query := "DELETE FROM users WHERE id = ?"
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
	query := "SELECT id, name, email, phone, password FROM users WHERE email = ? OR phone = ?"
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
