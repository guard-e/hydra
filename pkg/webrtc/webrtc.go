package webrtc

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

// CallManager управляет WebRTC звонками
type CallManager struct {
	mu          sync.Mutex
	activeCalls map[string]*CallSession
	iceServers  []webrtc.ICEServer
}

// CallSession представляет активный звонок
type CallSession struct {
	ID          string
	PeerConn    *webrtc.PeerConnection
	AudioTrack  *webrtc.TrackLocalStaticSample
	IsInitiator bool
	CreatedAt   time.Time
	mu          sync.Mutex
}

// CallOffer содержит данные для установки звонка
type CallOffer struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

// CallAnswer содержит ответ на звонок
type CallAnswer struct {
	SDP  string `json:"sdp"`
	Type string `json:"type"`
}

// NewCallManager создает новый менеджер звонков
func NewCallManager(iceServersURLs []string) *CallManager {
	if len(iceServersURLs) == 0 {
		iceServersURLs = []string{"stun:stun.l.google.com:19302"}
	}
	return &CallManager{
		activeCalls: make(map[string]*CallSession),
		iceServers: []webrtc.ICEServer{
			{
				URLs: iceServersURLs,
			},
		},
	}
}

// CreateOffer создает предложение для нового звонка
func (cm *CallManager) CreateOffer(ctx context.Context, callID string) (*CallOffer, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Создаем peer connection
	config := webrtc.Configuration{
		ICEServers: cm.iceServers,
	}

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	// Создаем аудио трек
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"hydra-audio",
	)
	if err != nil {
		peerConnection.Close()
		return nil, fmt.Errorf("failed to create audio track: %w", err)
	}

	// Добавляем трек в соединение
	_, err = peerConnection.AddTrack(audioTrack)
	if err != nil {
		peerConnection.Close()
		return nil, fmt.Errorf("failed to add audio track: %w", err)
	}

	// Обработчики событий соединения
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("Call %s connection state: %s", callID, s.String())
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateDisconnected {
			cm.cleanupCall(callID)
		}
	})

	peerConnection.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		log.Printf("Call %s ICE connection state: %s", callID, s.String())
	})

	// Создаем предложение
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		peerConnection.Close()
		return nil, fmt.Errorf("failed to create offer: %w", err)
	}

	// Устанавливаем локальное описание
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		peerConnection.Close()
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	// Сохраняем сессию
	session := &CallSession{
		ID:          callID,
		PeerConn:    peerConnection,
		AudioTrack:  audioTrack,
		IsInitiator: true,
		CreatedAt:   time.Now(),
	}

	cm.activeCalls[callID] = session

	return &CallOffer{
		SDP:  offer.SDP,
		Type: offer.Type.String(),
	}, nil
}

// HandleAnswer обрабатывает ответ на звонок
func (cm *CallManager) HandleAnswer(ctx context.Context, callID string, answer CallAnswer) error {
	cm.mu.Lock()
	session, exists := cm.activeCalls[callID]
	cm.mu.Unlock()

	if !exists {
		return fmt.Errorf("call session not found")
	}

	// Преобразуем ответ в нужный формат
	answerSD := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answer.SDP,
	}

	// Устанавливаем удаленное описание
	return session.PeerConn.SetRemoteDescription(answerSD)
}

// CreateAnswer создает ответ на входящий звонок
func (cm *CallManager) CreateAnswer(ctx context.Context, callID string, offer CallOffer) (*CallAnswer, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Создаем peer connection
	config := webrtc.Configuration{
		ICEServers: cm.iceServers,
	}

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	// Создаем аудио трек
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"hydra-audio",
	)
	if err != nil {
		peerConnection.Close()
		return nil, fmt.Errorf("failed to create audio track: %w", err)
	}

	// Добавляем трек в соединение
	_, err = peerConnection.AddTrack(audioTrack)
	if err != nil {
		peerConnection.Close()
		return nil, fmt.Errorf("failed to add audio track: %w", err)
	}

	// Обработчики событий
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("Call %s connection state: %s", callID, s.String())
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateDisconnected {
			cm.cleanupCall(callID)
		}
	})

	// Устанавливаем удаленное описание (предложение)
	offerSD := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	}

	err = peerConnection.SetRemoteDescription(offerSD)
	if err != nil {
		peerConnection.Close()
		return nil, fmt.Errorf("failed to set remote description: %w", err)
	}

	// Создаем ответ
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		peerConnection.Close()
		return nil, fmt.Errorf("failed to create answer: %w", err)
	}

	// Устанавливаем локальное описание
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		peerConnection.Close()
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	// Сохраняем сессию
	session := &CallSession{
		ID:          callID,
		PeerConn:    peerConnection,
		AudioTrack:  audioTrack,
		IsInitiator: false,
		CreatedAt:   time.Now(),
	}

	cm.activeCalls[callID] = session

	return &CallAnswer{
		SDP:  answer.SDP,
		Type: answer.Type.String(),
	}, nil
}

// GetAudioTrack возвращает аудио трек для звонка
func (cm *CallManager) GetAudioTrack(callID string) (*webrtc.TrackLocalStaticSample, error) {
	cm.mu.Lock()
	session, exists := cm.activeCalls[callID]
	cm.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("call session not found")
	}

	return session.AudioTrack, nil
}

// EndCall завершает звонок
func (cm *CallManager) EndCall(callID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if session, exists := cm.activeCalls[callID]; exists {
		session.PeerConn.Close()
		delete(cm.activeCalls, callID)
		log.Printf("Call %s ended", callID)
	}
}

// IsCallActive проверяет активен ли звонок
func (cm *CallManager) IsCallActive(callID string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	_, exists := cm.activeCalls[callID]
	return exists
}

// cleanupCall очищает ресурсы звонка
func (cm *CallManager) cleanupCall(callID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if session, exists := cm.activeCalls[callID]; exists {
		session.PeerConn.Close()
		delete(cm.activeCalls, callID)
		log.Printf("Cleaned up call %s", callID)
	}
}

// GetActiveCalls возвращает список активных звонков
func (cm *CallManager) GetActiveCalls() []string {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var calls []string
	for callID := range cm.activeCalls {
		calls = append(calls, callID)
	}
	return calls
}
