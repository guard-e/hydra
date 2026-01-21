package server

import (
	"encoding/json"
	"fmt"
	"hydra/pkg/storage"
	"hydra/pkg/transport/manager"
	"hydra/pkg/voice"
	"hydra/pkg/webrtc"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
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
	transportManager *manager.TransportManager
	voiceProcessor   *voice.VoiceProcessor
	callManager      *webrtc.CallManager
	db               *storage.Storage
	contacts         map[string]Contact
	mu               sync.Mutex
}

func New(tm *manager.TransportManager, db *storage.Storage) *Server {
	// Создаем процессор голосовых сообщений
	voiceProcessor := voice.New(tm, "./voice_storage")

	// Создаем менеджер звонков
	callManager := webrtc.NewCallManager()

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
		transportManager: tm,
		voiceProcessor:   voiceProcessor,
		callManager:      callManager,
		db:               db,
		contacts:         make(map[string]Contact),
	}
}

func (s *Server) Start(addr string) error {
	http.Handle("/", http.FileServer(http.Dir("./web")))
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
	file, header, err := r.FormFile("audio")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "No audio file provided: " + err.Error()})
		return
	}
	defer file.Close()

	// Можно получить поле 'to'
	to := r.FormValue("to")
	log.Printf("Received voice message for %s", to)

	// Создаем временный файл для обработки
	tempFile, err := os.CreateTemp("", "voice_upload_*.webm")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create temp file: " + err.Error()})
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Копируем данные во временный файл
	if _, err := io.Copy(tempFile, file); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to save audio: " + err.Error()})
		return
	}

	// Создаем FileHeader для обработки
	fileHeader := &multipart.FileHeader{
		Filename: header.Filename,
		Header:   header.Header,
		Size:     header.Size,
	}

	// Обрабатываем голосовое сообщение
	voiceMsg, err := s.voiceProcessor.Record(r.Context(), fileHeader)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to process voice: " + err.Error(),
		})
		return
	}

	// Отправляем через транспорт
	// В будущем здесь будет маршрутизация на основе 'to'
	if err := s.voiceProcessor.Send(r.Context(), voiceMsg); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to send voice: " + err.Error(),
		})
		return
	}

	response := map[string]interface{}{
		"success":  true,
		"id":       voiceMsg.ID,
		"duration": voiceMsg.Duration,
	}

	json.NewEncoder(w).Encode(response)
}

// handleVoiceGet возвращает аудио файл
func (s *Server) handleVoiceGet(w http.ResponseWriter, r *http.Request) {
	// При ошибках возвращаем JSON, при успехе - файл
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	// Извлекаем ID из URL
	voiceID := strings.TrimPrefix(r.URL.Path, "/api/voice/")
	if voiceID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Voice ID required"})
		return
	}

	filePath, err := s.voiceProcessor.GetVoiceMessagePathByID(voiceID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	http.ServeFile(w, r, filePath)
}

// handleCallStart начинает новый звонок
func (s *Server) handleCallStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		CallID string `json:"call_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON: " + err.Error()})
		return
	}

	if req.CallID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Call ID required"})
		return
	}

	// Создаем предложение для звонка
	offer, err := s.callManager.CreateOffer(r.Context(), req.CallID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create call offer: " + err.Error()})
		return
	}

	response := map[string]interface{}{
		"success": true,
		"call_id": req.CallID,
		"offer":   offer,
	}

	json.NewEncoder(w).Encode(response)
}

// handleCallAnswer обрабатывает ответ на звонок
func (s *Server) handleCallAnswer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		CallID string            `json:"call_id"`
		Answer webrtc.CallAnswer `json:"answer"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON: " + err.Error()})
		return
	}

	if req.CallID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Call ID required"})
		return
	}

	// Обрабатываем ответ
	err := s.callManager.HandleAnswer(r.Context(), req.CallID, req.Answer)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to handle call answer: " + err.Error()})
		return
	}

	response := map[string]interface{}{
		"success": true,
		"call_id": req.CallID,
	}

	json.NewEncoder(w).Encode(response)
}

// handleCallOffer обрабатывает входящее предложение звонка
func (s *Server) handleCallOffer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		CallID string           `json:"call_id"`
		Offer  webrtc.CallOffer `json:"offer"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON: " + err.Error()})
		return
	}

	if req.CallID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Call ID required"})
		return
	}

	// Создаем ответ на предложение
	answer, err := s.callManager.CreateAnswer(r.Context(), req.CallID, req.Offer)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create call answer: " + err.Error()})
		return
	}

	response := map[string]interface{}{
		"success": true,
		"call_id": req.CallID,
		"answer":  answer,
	}

	json.NewEncoder(w).Encode(response)
}

// handleCallEnd завершает звонок
func (s *Server) handleCallEnd(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		CallID string `json:"call_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON: " + err.Error()})
		return
	}

	if req.CallID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Call ID required"})
		return
	}

	// Завершаем звонок
	s.callManager.EndCall(req.CallID)

	response := map[string]interface{}{
		"success": true,
		"call_id": req.CallID,
	}

	json.NewEncoder(w).Encode(response)
}

// handleCallStatus возвращает статус звонков
func (s *Server) handleCallStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	activeCalls := s.callManager.GetActiveCalls()

	response := map[string]interface{}{
		"success":      true,
		"active_calls": activeCalls,
		"total_calls":  len(activeCalls),
	}

	json.NewEncoder(w).Encode(response)
}
