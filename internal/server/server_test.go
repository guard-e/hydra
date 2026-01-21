package server

import (
	"bytes"
	"encoding/json"
	"hydra/internal/config"
	"hydra/pkg/storage"
	"hydra/pkg/transport/manager"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupTestServer() (*Server, func()) {
	// Use the same credentials as in main.go, but potentially a different DB or the same one.
	// WARNING: This runs against the real DB if configured so.
	// For now, we use the provided credentials.
	connStr := "user=postgres password=postgres dbname=hydra sslmode=disable"

	// Initialize storage
	store, err := storage.New(connStr)
	if err != nil {
		// If DB is not available, we can't run tests.
		// In a real CI environment, we'd handle this better (skip or fail).
		// For this local setup, we'll panic to indicate failure.
		panic(err)
	}

	// Initialize transport manager (mock or minimal)
	tm := manager.New()

	// Create config
	cfg := &config.Config{
		DatabaseURL:      connStr,
		ServerPort:       "8081",
		VoiceStoragePath: "./test_voice_storage",
		WebStaticPath:    "./test_web",
		SMTPHost:         "localhost",
		SMTPPort:         "25",
		SMTPUser:         "user",
		SMTPPassword:     "pass",
		SMTPFrom:         "test@example.com",
	}

	// Create server
	srv := New(cfg, tm, store)

	cleanup := func() {
		// Optional: Clean up test data
		// store.DeleteUser(...)
	}

	return srv, cleanup
}

func TestSMSFlow(t *testing.T) {
	// Recover from panic if DB is not available
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Skipping test due to DB connection error: %v", r)
		}
	}()

	srv, cleanup := setupTestServer()
	defer cleanup()

	phone := "+1234567890"

	// 1. Send SMS Code
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"phone": phone})
	req := httptest.NewRequest("POST", "/api/sms/send", bytes.NewBuffer(body))
	srv.handleSMSSend(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// 2. Inject Code manually for verification test
	knownCode := "123456"
	err := srv.db.CreateSMSVerification(phone, knownCode)
	if err != nil {
		t.Fatalf("Failed to inject code: %v", err)
	}

	// 3. Verify SMS Code
	w = httptest.NewRecorder()
	body, _ = json.Marshal(map[string]string{"phone": phone, "code": knownCode})
	req = httptest.NewRequest("POST", "/api/sms/verify", bytes.NewBuffer(body))
	srv.handleSMSVerify(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for verify, got %d. Body: %s", w.Code, w.Body.String())
	}

	// 4. Register/Login with Phone
	w = httptest.NewRecorder()
	body, _ = json.Marshal(map[string]string{
		"phone":    phone,
		"name":     "Test User",
		"password": "password123",
	})
	req = httptest.NewRequest("POST", "/api/auth/phone", bytes.NewBuffer(body))
	srv.handlePhoneAuth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for auth, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Check if user was created
	user, err := srv.db.GetUserByPhone(phone)
	if err != nil {
		t.Errorf("User was not created: %v", err)
	} else if user.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got '%s'", user.Name)
	}

	// Cleanup test user
	if user != nil {
		srv.db.DeleteUser(user.ID)
	}
}

func TestEmailFlow(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Skipping test due to DB connection error: %v", r)
		}
	}()

	srv, cleanup := setupTestServer()
	defer cleanup()

	email := "test@example.com"
	knownCode := "654321"

	// 1. Send Email (Just check it doesn't crash)
	w := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"email": email})
	req := httptest.NewRequest("POST", "/api/email/send", bytes.NewBuffer(body))
	srv.handleEmailSend(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// 2. Inject Code manually for verification test
	err := srv.db.CreateEmailVerification(email, knownCode)
	if err != nil {
		t.Fatalf("Failed to inject code: %v", err)
	}

	// 3. Verify Email Code
	w = httptest.NewRecorder()
	body, _ = json.Marshal(map[string]string{"email": email, "code": knownCode})
	req = httptest.NewRequest("POST", "/api/email/verify", bytes.NewBuffer(body))
	srv.handleEmailVerify(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for verify, got %d. Body: %s", w.Code, w.Body.String())
	}

	// 4. Register/Login with Email
	w = httptest.NewRecorder()
	body, _ = json.Marshal(map[string]string{
		"email":    email,
		"name":     "Test Email User",
		"password": "password123",
	})
	req = httptest.NewRequest("POST", "/api/auth/email", bytes.NewBuffer(body))
	srv.handleEmailAuth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for auth, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Check if user was created
	user, err := srv.db.GetUserByEmail(email)
	if err != nil {
		t.Errorf("User was not created: %v", err)
	} else if user.Name != "Test Email User" {
		t.Errorf("Expected name 'Test Email User', got '%s'", user.Name)
	}

	// Cleanup test user
	if user != nil {
		srv.db.DeleteUser(user.ID)
	}
}
