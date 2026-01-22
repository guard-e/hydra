# Руководство по развертыванию Hydra Messenger

В этом документе описан процесс развертывания сервера Hydra Messenger на публичном хостинге (VPS).

## Требования

- **VPS (Virtual Private Server)**:
  - OS: Ubuntu 22.04 / Debian 11+ (рекомендуется)
  - CPU: 1 Core
  - RAM: 1 GB (минимум), 2 GB (рекомендуется)
  - Disk: 10 GB+
- **PostgreSQL 15+**
- **Доменное имя** (опционально, но рекомендуется для HTTPS)

---

## Конфигурация

Все настройки приложения вынесены в файл `.env`.
Перед запуском (любым способом) создайте файл конфигурации:

```bash
cp .env.example .env
```

Отредактируйте файл `.env` в соответствии с вашими требованиями:

- **DATABASE_URL**: Строка подключения к PostgreSQL.
  - Для локального запуска: `postgres://postgres:postgres@localhost:5432/hydra?sslmode=disable`
  - Для Docker: обычно `postgres://postgres:postgres@db:5432/hydra?sslmode=disable` (хост `db`)
- **SERVER_PORT**: Порт сервера (по умолчанию 8081).
- **SMTP_***: Настройки почты для отправки кодов подтверждения.
  - **Важно для Mail.ru/Yandex/Gmail**: Используйте "Пароль приложений" (App Password), а не основной пароль от аккаунта.
  - Для Mail.ru: `SMTP_HOST=smtp.mail.ru`, `SMTP_PORT=465` (SSL/TLS).
- **SMS_***: Настройки для отправки SMS (опционально).
  - `SMS_PROVIDER`: `console` (для тестов, вывод в лог) или `http` (для внешнего API).
  - `SMS_API_URL`: URL API для отправки (только для `http`).
  - `SMS_API_KEY`: API ключ (только для `http`).
- **ICE_SERVERS**: STUN/TURN серверы для звонков.
- **Пути**: Пути к статике и хранилищу голоса.

---

## Вариант 1: Развертывание через Docker (Рекомендуется)

Это самый простой и надежный способ.

### 1. Установка Docker

На вашем VPS выполните следующие команды:

```bash
# Обновляем пакеты
sudo apt update && sudo apt upgrade -y

# Устанавливаем Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Добавляем текущего пользователя в группу docker
sudo usermod -aG docker $USER
newgrp docker
```

### 2. Загрузка проекта

Клонируйте репозиторий:

```bash
git clone https://github.com/guard-e/hydra.git
cd hydra
```

### 3. Запуск

Используйте `docker-compose` для сборки и запуска:

```bash
docker compose up -d --build
```

Сервер будет доступен по адресу: `http://<IP-вашего-сервера>:8081`

Для просмотра логов:
```bash
docker compose logs -f
```

---

## Вариант 2: Ручная установка (Linux)

### 1. Установка Go

```bash
wget https://go.dev/dl/go1.25.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.25.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### 2. Установка PostgreSQL

```bash
sudo apt install postgresql postgresql-contrib -y
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

Настройка базы данных (пользователь `postgres`, пароль `postgres`, база `hydra`):

```bash
sudo -u postgres psql -c "ALTER USER postgres WITH PASSWORD 'postgres';"
sudo -u postgres psql -c "CREATE DATABASE hydra;"
```

### 3. Сборка

```bash
# В директории проекта
go mod download
go build -o hydra-server cmd/hydra/main.go
```

### 4. Настройка Systemd (автозапуск)

Создайте файл службы:

```bash
sudo nano /etc/systemd/system/hydra.service
```

Вставьте следующее содержимое (замените пути на свои):

```ini
[Unit]
Description=Hydra Messenger Server
After=network.target

[Service]
User=root
WorkingDirectory=/root/hydra
ExecStart=/root/hydra/hydra-server
Restart=always
RestartSec=5
# Если переменные окружения не подтягиваются из .env файла автоматически,
# можно раскомментировать следующую строку (требует полного пути):
# EnvironmentFile=/root/hydra/.env

[Install]
WantedBy=multi-user.target
```

Запустите службу:

```bash
sudo systemctl daemon-reload
sudo systemctl enable hydra
sudo systemctl start hydra
```

---

## Настройка Nginx и SSL (HTTPS)

Для безопасного доступа рекомендуется использовать Nginx как обратный прокси с SSL сертификатом от Let's Encrypt.

### 1. Установка Nginx и Certbot

```bash
sudo apt install nginx certbot python3-certbot-nginx -y
```

### 2. Настройка Nginx

Создайте конфиг:

```bash
sudo nano /etc/nginx/sites-available/hydra
```

Содержимое:

```nginx
server {
    server_name your-domain.com; # Замените на ваш домен

    location / {
        proxy_pass http://localhost:8081;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}
```

Активируйте конфиг:

```bash
sudo ln -s /etc/nginx/sites-available/hydra /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### 3. Получение SSL сертификата

```bash
sudo certbot --nginx -d your-domain.com
```

Теперь ваш сервер будет доступен по `https://your-domain.com`.

---

## Настройка Firewall (UFW)

Не забудьте открыть необходимые порты:

```bash
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 80/tcp    # HTTP (для Certbot)
sudo ufw allow 443/tcp   # HTTPS
sudo ufw allow 8080      # Mesh Transport (P2P)
sudo ufw enable
```

*Примечание: Порт 8081 открывать наружу не нужно, если вы используете Nginx.*
