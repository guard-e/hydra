package discovery

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
)

// ServiceDiscovery управляет автоматическим обнаружением пиров через mDNS
type ServiceDiscovery struct {
	serviceName string
	port        int
	peers       map[string]string // peerID -> address
	mu          sync.RWMutex
	stopChan    chan struct{}
}

func New(serviceName string, port int) *ServiceDiscovery {
	return &ServiceDiscovery{
		serviceName: serviceName,
		port:        port,
		peers:       make(map[string]string),
		stopChan:    make(chan struct{}),
	}
}

// Start запускает mDNS сервер для анонса и обнаружения сервисов
func (sd *ServiceDiscovery) Start() error {
	// Получаем локальный IP для анонса
	localIP, err := getLocalIP()
	if err != nil {
		return fmt.Errorf("failed to get local IP: %v", err)
	}

	// Анонсируем наш сервис
	if err := sd.advertiseService(localIP); err != nil {
		return fmt.Errorf("failed to advertise service: %v", err)
	}

	// Запускаем обнаружение других сервисов
	go sd.discoverServices()

	log.Printf("mDNS discovery started. Service: %s, Port: %d", sd.serviceName, sd.port)
	return nil
}

// Stop останавливает обнаружение
func (sd *ServiceDiscovery) Stop() {
	close(sd.stopChan)
}

// GetPeers возвращает список обнаруженных пиров
func (sd *ServiceDiscovery) GetPeers() []string {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	peers := make([]string, 0, len(sd.peers))
	for _, addr := range sd.peers {
		peers = append(peers, addr)
	}
	return peers
}

// advertiseService анонсирует наш сервис через mDNS
func (sd *ServiceDiscovery) advertiseService(ip string) error {
	// Создаем mDNS сервер для анонса
	service, err := mdns.NewMDNSService(
		"Hydra Messenger",
		sd.serviceName,
		"",
		"",
		sd.port,
		[]net.IP{net.ParseIP(ip)},
		[]string{"txtv=1", "type=messenger"},
	)
	if err != nil {
		return err
	}

	server, err := mdns.NewServer(&mdns.Config{
		Zone: service,
	})
	if err != nil {
		return err
	}

	// Сервер будет работать в фоне
	defer server.Shutdown()

	return nil
}

// discoverServices ищет другие сервисы в сети
func (sd *ServiceDiscovery) discoverServices() {
	entries := make(chan *mdns.ServiceEntry)

	// Параметры поиска
	params := mdns.QueryParam{
		Service:             sd.serviceName,
		Domain:              "local",
		Timeout:             10 * time.Second,
		Entries:             entries,
		WantUnicastResponse: false,
	}

	// Периодический поиск
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sd.stopChan:
			return
		case <-ticker.C:
			go func() {
				if err := mdns.Query(&params); err != nil {
					log.Printf("mDNS query error: %v", err)
				}
			}()
		case entry := <-entries:
			if entry.AddrV4 != nil {
				peerAddr := fmt.Sprintf("%s:%d", entry.AddrV4.String(), entry.Port)

				sd.mu.Lock()
				sd.peers[entry.Name] = peerAddr
				sd.mu.Unlock()

				log.Printf("Discovered peer: %s (%s)", entry.Name, peerAddr)
			}
		}
	}
}

// getLocalIP возвращает локальный IP адрес
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no local IP found")
}
