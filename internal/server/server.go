package server

import (
	"encoding/json"
	"fmt"
	"hydra/internal/config"
	"hydra/pkg/storage"
	"hydra/pkg/transport/manager"
	"hydra/pkg/voice"
	"hydra/pkg/webrtc"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

type Contact struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Status string `json:"status"`
}

type Server struct {
	config           *config.Config
	transportManager *manager.TransportManager
	voiceProcessor   *voice.VoiceProcessor
	callManager      *webrtc.CallManager
	db               *storage.Storage
	contacts         map[string]Contact
	mu               sync.Mutex
}

func New(cfg *config.Config, tm *manager.TransportManager, db *storage.Storage) *Server {
	// Создаем процессор голосовых сообщений
	voiceProcessor := voice.New(tm, "./voice_storage")

	// Создаем менеджер звонков
	callManager := webrtc.NewCallManager(cfg.ICEServers)

	// Запускаем очистку старых файлов каждые 24 часа
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			<-ticker.C
			voiceProcessor.Cleanup(7 * 24 * time.Hour) // Удаляем файлы старше 7 дней
		}
	}()

	return &Server{
		config:           cfg,
		transportManager: tm,
		voiceProcessor:   voiceProcessor,
		callManager:      callManager,
		db:               db,
		contacts:         make(map[string]Contact),
	}
}

func (s *Server) Start(addr string) error {
	http.Handle("/", http.FileServer(http.Dir(s.config.WebStaticPath)))
	http.HandleFunc("/api/contacts", s.handleContacts)
	http.HandleFunc("/api/send", s.handleSend)
	http.HandleFunc("/api/status", s.handleStatus)
	http.HandleFunc("/api/voice/send", s.handleVoiceSend)
	http.HandleFunc("/api/voice/", s.handleVoiceGet)
	http.HandleFunc("/api/call/start", s.handleCallStart)
	http.HandleFunc("/api/call/answer", s.handleCallAnswer)
	http.HandleFunc("/api/call/offer", s.handleCallOffer)
	http.HandleFunc("/api/call/end", s.handleCallEnd)
	http.HandleFunc("/api/call/status", s.handleCallStatus)
	http.HandleFunc("/api/invite", s.handleInvite)
	http.HandleFunc("/api/register", s.handleRegister)
	http.HandleFunc("/api/login", s.handleLogin)
	http.HandleFunc("/api/users/", s.handleUser)
	http.HandleFunc("/api/sms/send", s.handleSMSSend)
	http.HandleFunc("/api/sms/verify", s.handleSMSVerify)
	http.HandleFunc("/api/auth/phone", s.handlePhoneAuth)
	http.HandleFunc("/api/email/send", s.handleEmailSend)
	http.HandleFunc("/api/email/verify", s.handleEmailVerify)
	http.HandleFunc("/api/auth/email", s.handleEmailAuth)

	log.Printf("Web Interface started at http://localhost%s", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		ContactInfo string `json:"contact_info"`
		Password    string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	user, err := s.db.ValidateUser(req.ContactInfo, req.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid credentials"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"user":    user,
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	contactInfo, err := s.db.ValidateInvite(req.Token)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid or expired token"})
		return
	}

	user, err := s.db.CreateUser(req.Name, req.Password, contactInfo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create user"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"user":    user,
	})
}

func (s *Server) handleInvite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Email string `json:"email"`
		Phone string `json:"phone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	contactInfo := req.Email
	if contactInfo == "" {
		contactInfo = req.Phone
	}

	if contactInfo == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Email or phone required"})
		return
	}

	token, err := s.db.CreateInvite(contactInfo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create invite"})
		return
	}

	inviteLink := fmt.Sprintf("http://localhost:8081/register.html?token=%s", token)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"token":       token,
		"invite_link": inviteLink,
	})
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := strings.TrimPrefix(r.URL.Path, "/api/users/")

	switch r.Method {
	case http.MethodGet:
		user, err := s.db.GetUser(id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "User not found"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "user": user})

	case http.MethodPut:
		var user storage.User
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
			return
		}
		user.ID = id
		if err := s.db.UpdateUser(&user); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to update user"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	case http.MethodDelete:
		if err := s.db.DeleteUser(id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to delete user"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
	}
}

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		s.mu.Lock()
		defer s.mu.Unlock()

		list := make([]Contact, 0, len(s.contacts))
		for _, c := range s.contacts {
			list = append(list, c)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"contacts": list,
		})
		return
	}

	if r.Method == http.MethodPost {
		var req Contact
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
			return
		}

		if req.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Name required"})
			return
		}

		if req.ID == "" {
			req.ID = fmt.Sprintf("user%d", time.Now().UnixNano())
		}
		if req.Avatar == "" {
			req.Avatar = "#999999"
		}
		if req.Status == "" {
			req.Status = "offline"
		}

		s.mu.Lock()
		s.contacts[req.ID] = req
		s.mu.Unlock()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"contact": req,
		})
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
}

type sendRequest struct {
	Message string `json:"message"`
	To      string `json:"to"`
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request body"})
		return
	}

	if req.Message == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Message cannot be empty"})
		return
	}

	log.Printf("Received message from UI: %s to %s", req.Message, req.To)

	// Отправляем через менеджер транспортов (автоматическое переключение)
	// В будущем можно использовать req.To для маршрутизации
	err := s.transportManager.Send(r.Context(), []byte(req.Message))

	// Получаем текущий активный транспорт для статуса
	currentTransport := s.transportManager.GetCurrentTransport()

	response := map[string]interface{}{
		"success":   true,
		"transport": currentTransport.Name(),
	}

	if err != nil {
		log.Printf("Transport error: %v", err)
		response["success"] = false
		response["error"] = err.Error()
		// Не возвращаем 500, так как это ошибка транспорта, а не сервера
	}

	json.NewEncoder(w).Encode(response)
}

// SMS Verification Handlers
func (s *Server) handleSMSSend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Phone string `json:"phone"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	// Генерируем 6-значный код
	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)

	// Сохраняем код в базу данных
	if err := s.db.CreateSMSVerification(req.Phone, code); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create verification code"})
		return
	}

	// В реальном приложении здесь должен быть вызов SMS-сервиса
	log.Printf("SMS verification code for %s: %s", req.Phone, code)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Verification code sent",
	})
}

func (s *Server) handleSMSVerify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	// Проверяем код
	valid, err := s.db.ValidateSMSVerification(req.Phone, req.Code)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	if !valid {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid verification code"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Phone number verified successfully",
	})
}

func (s *Server) handleEmailSend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)

	if err := s.db.CreateEmailVerification(req.Email, code); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create verification code"})
		return
	}

	// Send Email
	if s.config.SMTPHost != "" && s.config.SMTPUser != "" {
		go func() {
			err := s.sendEmail(req.Email, "Hydra Verification Code", fmt.Sprintf("Your verification code is: %s", code))
			if err != nil {
				log.Printf("Failed to send email to %s: %v", req.Email, err)
			}
		}()
	} else {
		log.Printf("Email config missing. Code for %s: %s", req.Email, code)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Verification code sent",
	})
}

func (s *Server) sendEmail(to, subject, body string) error {
	auth := smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPassword, s.config.SMTPHost)
	msg := []byte(fmt.Sprintf("To: %s\r\n"+
		"Subject: %s\r\n"+
		"\r\n"+
		"%s\r\n", to, subject, body))

	addr := fmt.Sprintf("%s:%s", s.config.SMTPHost, s.config.SMTPPort)
	return smtp.SendMail(addr, auth, s.config.SMTPFrom, []string{to}, msg)
}

func (s *Server) handleEmailVerify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	valid, err := s.db.ValidateEmailVerification(req.Email, req.Code)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	if !valid {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid verification code"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Email verified successfully",
	})
}

func (s *Server) handlePhoneAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Phone    string `json:"phone"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	// Проверяем, существует ли пользователь с таким номером
	existingUser, err := s.db.GetUserByPhone(req.Phone)
	if err == nil {
		// Пользователь существует - выполняем вход
		if existingUser.Password != req.Password {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid password"})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"user":    existingUser,
			"message": "Login successful",
		})
		return
	}

	// Пользователь не существует - создаем нового
	user, err := s.db.CreateUser(req.Name, req.Password, req.Phone)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create user"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"user":    user,
		"message": "Registration successful",
	})
}

func (s *Server) handleEmailAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	existingUser, err := s.db.GetUserByEmail(req.Email)
	if err == nil {
		if existingUser.Password != req.Password {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid password"})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"user":    existingUser,
			"message": "Login successful",
		})
		return
	}

	user, err := s.db.CreateUser(req.Name, req.Password, req.Email)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create user"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"user":    user,
		"message": "Registration successful",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := s.transportManager.GetStatus()

	response := map[string]interface{}{
		"transports": status,
		"status":     "active",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVoiceSend обрабатывает отправку голосовых сообщений
func (s *Server) handleVoiceSend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	// Парсим multipart форму
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to parse form: " + err.Error()})
		return
	}

	// Получаем аудио файл
	_, header, err := r.FormFile("audio")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "No audio file provided: " + err.Error()})
		return
	}

	// Обрабатываем голосовое сообщение
	voiceMsg, err := s.voiceProcessor.Record(r.Context(), header)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to process voice message: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"voice_id": voiceMsg.ID,
		"duration": voiceMsg.Duration,
		"url":      fmt.Sprintf("/api/voice/%s.mp3", voiceMsg.ID),
	})
}

func (s *Server) handleVoiceGet(w http.ResponseWriter, r *http.Request) {
	voiceID := strings.TrimPrefix(r.URL.Path, "/api/voice/")
	voiceID = strings.TrimSuffix(voiceID, ".mp3")

	if voiceID == "" {
		http.Error(w, "Voice ID required", http.StatusBadRequest)
		return
	}

	filePath := fmt.Sprintf("./voice_storage/%s.mp3", voiceID)
	http.ServeFile(w, r, filePath)
}

func (s *Server) handleCallStart(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *Server) handleCallAnswer(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *Server) handleCallOffer(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *Server) handleCallEnd(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *Server) handleCallStatus(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
