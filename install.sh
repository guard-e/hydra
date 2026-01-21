#!/bin/bash

# Hydra Messenger Auto-Installer
# Автоматическая установка и настройка клиента

echo "🚀 Hydra Messenger Auto-Installer"
echo "=================================="

# Проверяем наличие Go
if ! command -v go &> /dev/null; then
    echo "❌ Go не установлен. Установите Go сначала: https://golang.org/dl/"
    exit 1
fi

# Проверяем версию Go
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
if [ "$(printf '%s\n' "1.18" "$GO_VERSION" | sort -V | head -n1)" != "1.18" ]; then
    echo "❌ Требуется Go версии 1.18 или выше. Текущая версия: $GO_VERSION"
    exit 1
fi

# Создаем директорию для установки
INSTALL_DIR="$HOME/hydra-messenger"
if [ ! -d "$INSTALL_DIR" ]; then
    mkdir -p "$INSTALL_DIR"
    echo "📁 Создана директория: $INSTALL_DIR"
fi

# Клонируем или обновляем репозиторий
if [ -d "$INSTALL_DIR/.git" ]; then
    echo "🔄 Обновление существующей установки..."
    cd "$INSTALL_DIR"
    git pull origin main
else
    echo "📥 Клонирование репозитория..."
    git clone https://github.com/your-repo/hydra.git "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# Устанавливаем зависимости
echo "📦 Установка зависимостей..."
go mod download

# Собираем бинарник
echo "🔨 Сборка бинарного файла..."
go build -o hydra-messenger cmd/hydra/main.go

# Создаем конфигурационный файл
if [ ! -f "config.yaml" ]; then
    cat > config.yaml << EOF
# Конфигурация Hydra Messenger
service:
  name: "hydra-messenger"
  port: 8080
  discovery: true

mesh:
  enabled: true
  autodiscovery: true
  static_peers: []

fronting:
  enabled: true
  cdn_domains:
    - "ajax.googleapis.com"
    - "cdn.cloudflare.com"
    - "d3a2p9q8.stackpathcdn.com"
    - "assets.buymeacoffee.com"
  hidden_domain: "secret-chat.appspot.com"
EOF
    echo "📝 Создан конфигурационный файл: config.yaml"
fi

# Создаем service файл для systemd (Linux)
if [ "$(uname)" = "Linux" ]; then
    SERVICE_FILE="/etc/systemd/system/hydra-messenger.service"
    if [ ! -f "$SERVICE_FILE" ]; then
        sudo tee "$SERVICE_FILE" > /dev/null << EOF
[Unit]
Description=Hydra Messenger Service
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/hydra-messenger
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
        echo "🔧 Создан systemd service файл"
        sudo systemctl daemon-reload
        sudo systemctl enable hydra-messenger
        echo "✅ Сервис добавлен в автозагрузку"
    fi
fi

# Создаем ярлык для Desktop (Windows)
if [ "$(uname)" = "MINGW"* ] || [ "$(uname)" = "CYGWIN"* ]; then
    SHORTCUT_FILE="$USERPROFILE/Desktop/Hydra Messenger.lnk"
    if [ ! -f "$SHORTCUT_FILE" ]; then
        powershell -Command "
        wsShell = New-Object -ComObject WScript.Shell
        shortcut = wsShell.CreateShortcut('$SHORTCUT_FILE')
        shortcut.TargetPath = '$(cygpath -w "$INSTALL_DIR/hydra-messenger.exe")'
        shortcut.WorkingDirectory = '$(cygpath -w "$INSTALL_DIR")'
        shortcut.Description = 'Hydra Messenger Client'
        shortcut.Save()
        "
        echo "🔗 Создан ярлык на рабочем столе"
    fi
fi

echo ""
echo "✅ Установка завершена!"
echo "📋 Что дальше:"
echo "   1. Запустите: $INSTALL_DIR/hydra-messenger"
echo "   2. Откройте: http://localhost:8080"
echo "   3. Подключите другие устройства в той же сети"
echo ""
echo "🌐 Автоматическое обнаружение включено!"
echo "   Другие клиенты в сети будут обнаружены автоматически"

# Запускаем сервис если systemd доступен
if [ -f "$SERVICE_FILE" ]; then
    echo ""
    read -p "🚀 Запустить сервис сейчас? (y/n): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo systemctl start hydra-messenger
        echo "▶️  Сервис запущен!"
    fi
fi