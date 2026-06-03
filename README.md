# vpnconfig

Автоматический обовлятор конфигурации sing-box на основе мультиссылок сервиса
[hynet.space](https://hynet.space).

## Назначение

Этот проект предназначен для автоматического обновления конфигурации
[sing-box](https://sing-box.sagernet.org/) на роутерах с OpenWRT, где sing-box
управляется плагином [podkop](https://podkop.net/).

Сервис hynet.space предоставляет мультиссылки (subscription URLs), содержащие
список VPN-серверов разных типов (VLESS, Trojan, Shadowsocks). Этот инструмент:

1. Загружает список серверов из hynet.space
2. Определяет страну каждого сервера по IP-адресу
3. Группирует серверы по странам
4. Обновляет секции конфигурации sing-box типа URLTest, созданные через интерфейс podkop

## Принцип работы

### Общий алгоритм

```
1. Загрузка ссылок из hynet.space
   └─ Ответ декодируется из base64

2. Обработка каждой ссылки:
   ├─ Извлечение IP-адреса из VPN-URL
   ├─ Пропуск vmess (не поддерживается)
   ├─ Определение страны через ip-api.com
   │  └─ Результат кешируется в файл
   └─ Парсинг VPN-URL в sing-box outbound

3. Загрузка конфигурации sing-box

4. Обновление секций:
   ├─ Удаление старых outbounds секции
   │  └─ Rulesets (local/remote) сохраняются
   ├─ Генерация новых outbounds:
   │  ├─ Прокси outbounds (section-N-out)
   │  ├─ URLTest outbound (section-urltest-out)
   │  └─ Selector outbound (section-out)
   └─ Добавление в конфигурацию

5. Сохранение обновлённой конфигурации
```

### Структура секции

Для каждой секции в конфигурации (`config.yaml`) генерируется группа outbounds:

**Пример для секции `MULTI_WEST` с 4 серверами:**

```json
[
  { "tag": "MULTI_WEST-1-out", "type": "vless", ... },
  { "tag": "MULTI_WEST-2-out", "type": "trojan", ... },
  { "tag": "MULTI_WEST-3-out", "type": "shadowsocks", ... },
  { "tag": "MULTI_WEST-4-out", "type": "vless", ... },
  {
    "tag": "MULTI_WEST-urltest-out",
    "type": "urltest",
    "outbounds": ["MULTI_WEST-1-out", "MULTI_WEST-2-out", "MULTI_WEST-3-out", "MULTI_WEST-4-out"],
    "url": "https://www.gstatic.com/generate_204",
    "interval": "3m",
    "tolerance": 50
  },
  {
    "tag": "MULTI_WEST-out",
    "type": "selector",
    "outbounds": ["MULTI_WEST-1-out", "MULTI_WEST-2-out", "MULTI_WEST-3-out", "MULTI_WEST-4-out", "MULTI_WEST-urltest-out"],
    "default": "MULTI_WEST-urltest-out"
  }
]
```

## Конфигурация

Файл `config.yaml`:

```yaml
happ_url: "https://hynet.space/s/YOUR_SUBSCRIPTION_ID"
cache_path: "/opt/cache/cache.json"
singbox_config: "/etc/sing-box/singbox.json"
backup_dir: "/opt/backups/sing-box"
max_backups: 10
max_cache_size_bytes: 10485760

urltest_defaults:
  url: "https://www.gstatic.com/generate_204"
  interval: "3m"
  tolerance: 50

sections:
  - name: "MULTI_WEST"
    countries:
      - "Lithuania"
      - "Netherlands"
      - "United States"
      - "Sweden"
  - name: "MULTI_RU"
    countries:
      - "Russia"
```

### Описание полей

| Поле | Тип | Описание |
|------|-----|----------|
| `happ_url` | string | URL мультиссылки hynet.space |
| `cache_path` | string | Путь к файлу кеша ip-api.com |
| `singbox_config` | string | Путь к файлу конфигурации sing-box |
| `backup_dir` | string | Директория для бэкапов |
| `max_backups` | int | Максимальное количество бэкапов (старые удаляются) |
| `max_cache_size_bytes` | int64 | Максимальный размер файла кеша в байтах (при превышении очищается) |
| `urltest_defaults` | object | Настройки URLTest по умолчанию |
| `sections` | array | Список секций для обновления |

### Секции

Каждая секция соответствует URLTest-группе в podkop:

```yaml
sections:
  - name: "MULTI_WEST"
    countries:
      - "Lithuania"
      - "Netherlands"
      - "United States"
      - "Sweden"
    # Опциональное переопределение urltest для секции:
    urltest:
      url: "https://custom-url.com"
      interval: "5m"
      tolerance: 100
```

**Поля секции:**

| Поле | Тип | Описание |
|------|-----|----------|
| `name` | string | Имя секции (должно совпадать с именем URLTest в podkop) |
| `countries` | array | Список стран, серверы из которых попадут в секцию |
| `urltest` | object | Опциональное переопределение настроек urltest для секции |

### URLTest в podkop

Секции в конфигурации vpnconfig соответствуют URLTest-секциям, созданным вручную
через интерфейс podkop (https://podkop.net/docs/sections/).

Важно: **имя секции в `config.yaml` должно совпадать** с именем URLTest-секции в
podkop. Например, если в podkop создана секция `MULTI_WEST`, то в `config.yaml`
должна быть соответствующая секция с `name: "MULTI_WEST"`.

При обновлении:
- Старые outbounds секции удаляются (кроме rulesets типа `local`/`remote`)
- Генерируются новые outbounds с актуальными серверами
- Генерируется urltest и selector для секции

## Использование

```bash

# Запуск
GOPROXY=direct go run ./cmd/updater/main.go

# Тесты
go test ./...
```

## Архитектура

```
cmd/updater/main.go              # Точка входа (инициализация зависимостей)
internal/
├── updater/
│   ├── updater.go               # Бизнес-логика обновления
│   └── updater_test.go          # Тесты
├── config/
│   └── config.go                # Загрузка конфигурации YAML
├── happ/
│   └── client.go                # Клиент hynet.space
├── http/
│   └── client.go                # HTTP-клиент с ротацией User-Agent
├── ipserv/
│   ├── client.go                # Клиент ip-api.com
│   ├── service.go               # Сервис с кешированием
│   └── cache.go                 # Файловый кеш
├── singbox/
│   └── config.go                # Манипуляции с конфигурацией sing-box
├── useragent/
│   └── useragent.go             # Генератор User-Agent
└── vpnurl/
    ├── parser.go                # Парсер VPN-URL
    ├── vless.go                 # Парсер VLESS
    ├── trojan.go                # Парсер Trojan
    └── shadowsocks.go           # Парсер Shadowsocks
```

## Поддерживаемые протоколы

- **VLESS** — с поддержкой Reality/TLS
- **Trojan** — с поддержкой TLS/WebSocket/gRPC
- **Shadowsocks** — с base64-кодированными credentials
- **vmess** — явно не поддерживается и пропускается

## Кеширование

Определение страны по IP кешируется в файл (`cache.json`) для уменьшения количества запросов к ip-api.com:

- Формат: `{"IP": {"country": "...", "timestamp": "..."}}`
- При превышении `max_cache_size_bytes` кеш очищается
- Кеш дополняется (read → append → write)

## Бэкапы

Перед каждым обновлением создаётся бэкап конфигурации sing-box:

- Формат имени: `singbox.json.backup_YYYYMMDD_HHMMSS`
- Хранятся в `backup_dir`
- При превышении `max_backups` удаляются самые старые

## Дисклеймеры

⚠️ **Этот проект полностью написан нейросетью Kimi K2.5.**

⚠️ **Автор имеет лишь поверхностную экспертизу в sing-box.** Проект создан экспериментальным путём, возможны ошибки в понимании внутренней логики sing-box и podkop.

⚠️ **Это личный проект для домашнего использования.** Не предназначен для production-использования без дополнительного тестирования. Используйте на свой страх и риск.

## Лицензия

MIT License. См. [LICENSE](LICENSE).
