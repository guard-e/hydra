package fronting

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"hydra/pkg/transport"
)

// Проверка соответствия интерфейсу
var _ transport.Transport = (*Transport)(nil)

// Transport реализует Domain Fronting.
type Transport struct {
	// FrontDomain - это домен, который мы "показываем" внешнему наблюдателю (например, SNI).
	// Это должен быть домен CDN или доверенного сервиса.
	FrontDomain string

	// HiddenDomain - это реальный домен нашего сервиса, который указывается в Host-заголовке.
	HiddenDomain string

	// EndpointUrl - полный URL для подключения (обычно https://FrontDomain/path).
	EndpointUrl string

	client *http.Client
}

// New создает новый экземпляр транспорта.
func New(frontDomain, hiddenDomain string) *Transport {
	// Создаем кастомный HTTP транспорт с оптимизированными настройками
	httpTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			// Ключевой момент 1: SNI (Server Name Indication) указывает на "белый" домен.
			ServerName: frontDomain,
		},
		// Оптимизированные таймауты
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   5,
		// Используем системные DNS с резервными серверами
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Пробуем несколько DNS серверов
			dnsServers := []string{"", "8.8.8.8:53", "1.1.1.1:53", "9.9.9.9:53"}

			for _, dnsServer := range dnsServers {
				var dialer net.Dialer
				if dnsServer != "" {
					dialer = net.Dialer{
						Timeout:   5 * time.Second,
						KeepAlive: 30 * time.Second,
						Resolver: &net.Resolver{
							PreferGo: true,
							Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
								return net.Dial("udp", dnsServer)
							},
						},
					}
				} else {
					dialer = net.Dialer{
						Timeout:   5 * time.Second,
						KeepAlive: 30 * time.Second,
					}
				}

				// Сначала устанавливаем TCP соединение
				tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
				if err != nil {
					continue
				}

				// Затем оборачиваем в TLS
				tlsConn := tls.Client(tcpConn, &tls.Config{
					ServerName: frontDomain,
				})

				// Выполняем handshake
				if err := tlsConn.HandshakeContext(ctx); err != nil {
					tcpConn.Close()
					continue
				}

				return tlsConn, nil
			}

			return nil, fmt.Errorf("all DNS servers failed for %s", addr)
		},
	}

	return &Transport{
		FrontDomain:  frontDomain,
		HiddenDomain: hiddenDomain,
		// По умолчанию стучимся на frontDomain.
		// Реальный роутинг произойдет на уровне CDN благодаря Host заголовку.
		EndpointUrl: fmt.Sprintf("https://%s/message", frontDomain),
		client: &http.Client{
			Transport: httpTransport,
			Timeout:   8 * time.Second, // Уменьшенный общий таймаут
		},
	}
}

func (t *Transport) Name() string {
	return "domain-fronting"
}

func (t *Transport) Connect(ctx context.Context) error {
	// HTTP stateless, явное соединение не требуется, но можно проверить доступность
	return nil
}

func (t *Transport) IsAvailable() bool {
	// В реальном сценарии здесь может быть ping-запрос
	return true
}

func (t *Transport) Send(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", t.EndpointUrl, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", t.EndpointUrl, err)
	}

	// Ключевой момент 2: Host заголовок указывает на скрытый сервис.
	// CDN увидит SNI=FrontDomain, но перенаправит запрос на HiddenDomain.
	req.Host = t.HiddenDomain

	// Добавляем заголовки, чтобы выглядеть как обычный трафик
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")

	// Добавляем таймаут для конкретного запроса
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := t.client.Do(req)
	if err != nil {
		// Анализируем тип ошибки для лучшего сообщения
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("request to %s timed out after 8 seconds", t.FrontDomain)
		}
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
			return fmt.Errorf("network connection failed to %s: %w", t.FrontDomain, err)
		}
		return fmt.Errorf("request to %s failed: %w", t.FrontDomain, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Читаем ошибку для отладки
		body, _ := io.ReadAll(resp.Body)

		// Специфичные коды ошибок CDN
		switch resp.StatusCode {
		case 403:
			return fmt.Errorf("CDN blocked request to %s (403 Forbidden)", t.FrontDomain)
		case 404:
			return fmt.Errorf("endpoint not found on %s (404 Not Found)", t.FrontDomain)
		case 502, 503, 504:
			return fmt.Errorf("CDN gateway error %d for %s", resp.StatusCode, t.FrontDomain)
		default:
			return fmt.Errorf("server %s returned status %d: %s", t.FrontDomain, resp.StatusCode, string(body))
		}
	}

	return nil
}
