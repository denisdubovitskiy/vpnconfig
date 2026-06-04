# vpnconfig

Автоматический обовлятор конфигурации sing-box на основе мультиссылок сервиса
[hynet.space](https://hynet.space).

## Назначение

Этот проект предназначен для автоматического обновления конфигурации
[sing-box](https://sing-box.sagernet.org/) на роутерах с OpenWRT, где sing-box
управляется плагином [podkop](https://podkop.net/).

Сервис hynet.space предоставляет мультиссылки (subscription URLs), содержащие
список VPN-серверов разных типов (VLESS, Trojan, Shadowsocks). Помимо этого
поддерживаются и обычные текстовые подписки. Этот инструмент:

1. Загружает список серверов из настроенных источников (happ/plaintext)
2. Определяет страну каждого сервера по IP-адресу
3. Группирует серверы по странам
4. Обновляет секции конфигурации sing-box типа URLTest, созданные через интерфейс podkop
5. Ведёт подробные логи каждого запуска для диагностики

## Принцип работы

### Общий алгоритм

```
1. Загрузка ссылок из настроенных источников каждой секции
   ├─ happ      — base64-encoded подписка hynet.space
   └─ plaintext — plain text подписка (одна ссылка на строку)

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

5. Проверка изменений:
   ├─ Если outbounds не изменились — сохранение пропускается
   └─ Если изменились — создание бэкапа и сохранение

6. Запись логов запуска
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
cache_path: "/opt/cache/cache.json"
singbox_config: "/etc/sing-box/singbox.json"
backup_dir: "/opt/backups/sing-box"
max_backups: 10
max_cache_size_bytes: 10485760
log_dir: "/var/log/vpnconfig"

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
    sources:
      - type: happ
        urls:
          - "https://hynet.space/s/YOUR_SUBSCRIPTION_ID_1"
          - "https://hynet.space/s/YOUR_SUBSCRIPTION_ID_2"
      - type: plaintext
        urls:
          - "https://raw.githubusercontent.com/user/repo/main/west/vless.txt"
  - name: "MULTI_RU"
    countries:
      - "Russia"
    sources:
      - type: happ
        urls:
          - "https://hynet.space/s/YOUR_SUBSCRIPTION_ID_RU"
      - type: plaintext
        urls:
          - "https://raw.githubusercontent.com/user/repo/main/ru/vless_ru.txt"

# Опционально: список провайдеров для определения страны по IP.
# Если не указан — используются все встроенные провайдеры в порядке fallback.
# Порядок в списке определяет приоритет: первый — самый предпочтительный.
# Доступные имена: ipapi_co, ip_api_com, ipwho_is, api_2ip_me, api_ip_sb, freegeoip_app
# geo_providers:
#   - ipapi_co
#   - ip_api_com
#   - ipwho_is

# Опционально: локальный MMDB-провайдер на базе MaxMind GeoLite2-Country.
# При первом запуске база будет скачана автоматически.
# mmdb:
#   enabled: true
#   database_path: "/opt/vpnconfig/GeoLite2-Country.mmdb"
#   # download_url: "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-Country.mmdb"

# Опционально: кастомные DNS-резолверы для определения IP по домену при
# парсинге VPN-ссылок. Порядок — fallback: первый предпочтительный, при
# сбое — второй, и т.д. Если не указано — используется системный
# резолвер (net.DefaultResolver).
#   - "https://host/path" — DNS over HTTPS (путь обязателен)
#   - "tls://host[:port]"  — DNS over TLS (порт по умолчанию 853)
#   - "host[:port]"        — обычный DNS через UDP/TCP (порт 53 по умолчанию)
# dns_resolvers:
#   - "https://cloudflare-dns.com/dns-query"
#   - "https://dns.google/dns-query"
#   - "tls://1.1.1.1"
#   - "1.1.1.1:53"
```

### Описание полей

| Поле | Тип | Описание |
|------|-----|----------|
| `cache_path` | string | Путь к файлу кеша ip-api.com |
| `singbox_config` | string | Путь к файлу конфигурации sing-box |
| `backup_dir` | string | Директория для бэкапов |
| `max_backups` | int | Максимальное количество бэкапов (старые удаляются) |
| `max_cache_size_bytes` | int64 | Максимальный размер файла кеша в байтах (при превышении очищается) |
| `log_dir` | string | Директория для файлов логов (опционально, см. раздел "Логирование") |
| `urltest_defaults` | object | Настройки URLTest по умолчанию |
| `sections` | array | Список секций для обновления |
| `geo_providers` | array | Список провайдеров для определения страны (опционально, см. ниже) |
| `mmdb` | object | Настройки локального MMDB-провайдера (опционально, см. ниже) |
| `singbox_cli` | object | Настройки CLI-проверки конфига (опционально, см. ниже) |
| `dns_resolvers` | array | Список кастомных DNS-резолверов (опционально, см. ниже) |

### Провайдеры для определения страны по IP

vpnconfig поддерживает несколько сервисов для определения страны по IP-адресу.
По умолчанию используются все встроенные провайдеры с **последовательным
fallback**: если первый провайдер недоступен, запрос автоматически идёт
ко второму, и так далее. Это обеспечивает устойчивость к rate limit
и временным сбоям отдельных сервисов.

| Имя | Сервис | Протокол | Возвращаемое поле |
|-----|--------|----------|-------------------|
| `ipapi_co` | ipapi.co | HTTPS (JSON) | `country` |
| `ip_api_com` | ip-api.com | HTTP (JSON) | `country` |
| `ipwho_is` | ipwho.is | HTTPS (JSON) | `country` |
| `api_2ip_me` | api.2ip.me | HTTPS (JSON) | `country` (англ.) |
| `api_ip_sb` | api.ip.sb | HTTPS (JSON) | `country` |
| `freegeoip_app` | freegeoip.app | HTTPS (JSON) | `country_name` |

Поле `geo_providers` опционально:
- Если не указано — используются все 6 провайдеров в порядке выше.
- Если указано — используется только перечисленный список, в указанном порядке.

Пример с кастомным порядком (только два провайдера):

```yaml
geo_providers:
  - ip_api_com      # сначала пробуем ip-api.com
  - ipwho_is        # при сбое — ipwho.is
```

### CLI-проверка конфигурации

Для предотвращения записи невалидной конфигурации sing-box можно включить
проверку через CLI-утилиту `sing-box`. При включении конфиг сначала сохраняется
во временный файл, проверяется командой `sing-box check`, и только после
успешной проверки заменяет оригинальный файл.

Провайдер опционален и активируется отдельной секцией `singbox_cli` в `config.yaml`.

```yaml
singbox_cli:
  enabled: true
  # cli_path: "sing-box"  # путь к утилите (по умолчанию "sing-box" из PATH)
```

**Поведение:**

- Если `enabled: true`, перед сохранением конфига выполняется проверка через `sing-box --config <tmp_path> check`.
- Если проверка не прошла — временный файл удаляется, оригинальный файл не изменяется.
- Если проверка прошла — временный файл заменяет оригинальный атомарно (через `os.Rename`).
- Если `cli_path` не указан, используется `sing-box` из PATH.

### Локальный MMDB-провайдер

Помимо онлайн-сервисов, vpnconfig умеет определять страну по IP через
**локальную базу MaxMind GeoLite2-Country** в формате MMDB. Работа полностью
офлайн, без обращения к внешним API.

Провайдер опционален и активируется отдельной секцией `mmdb` в `config.yaml`.

```yaml
mmdb:
  enabled: true
  database_path: "/opt/vpnconfig/GeoLite2-Country.mmdb"
  # download_url опционален — ниже показан дефолт.
  download_url: "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-Country.mmdb"
  # max_age опционален: файл старше этого значения будет перезагружен при старте.
  # Формат — Go duration string. По умолчанию 168h (1 неделя).
  # max_age: "720h"  # 30 дней
```

**Поведение:**

- Если `enabled: true`, MMDB-провайдер добавляется в начало списка — он
  проверяется первым, а HTTP-провайдеры становятся fallback'ом.
- Если файл `database_path` отсутствует или имеет нулевой размер, провайдер
  **скачает базу автоматически** по `download_url` (по умолчанию —
  `https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-Country.mmdb`).
- Если файл старше `max_age` (mtime), база **перезагружается автоматически**
  при старте. Старый файл удаляется, скачивается свежая версия.
  По умолчанию `max_age = 168h` (1 неделя) — MaxMind обновляет базу
  еженедельно, поэтому дефолт покрывает еженедельные релизы без ручной
  настройки.
- Все промежуточные директории в `database_path` создаются автоматически.
- Скачивание атомарно: данные пишутся во временный файл и переименовываются
  только при успешной записи.

**Поля `mmdb`:**

| Поле | Тип | Описание |
|------|-----|----------|
| `enabled` | bool | Включить MMDB-провайдер. По умолчанию `false`. |
| `database_path` | string | Путь к файлу `.mmdb`. **Обязателен** при `enabled: true`. |
| `download_url` | string | URL для скачивания. Опционально — по умолчанию зеркало P3TERX на GitHub. |
| `max_age` | duration | Максимально допустимый возраст файла. Если файл старше — перезагружается автоматически. Формат: Go duration string (`"720h"`, `"168h"`, `"30m"`). По умолчанию `168h` (1 неделя). |

**Пример с обоими типами провайдеров:**

```yaml
mmdb:
  enabled: true
  database_path: "/opt/vpnconfig/GeoLite2-Country.mmdb"

geo_providers:
  - ip_api_com      # fallback: HTTP-провайдер
  - ipwho_is
```

В этом случае порядок обработки IP: сначала MMDB (быстрый, офлайн), затем
ip-api.com, затем ipwho.is.

### Кастомные DNS-резолверы

По умолчанию vpnconfig использует системный DNS-резолвер
(`net.DefaultResolver`) для определения IP по доменному имени при парсинге
VPN-ссылок (например, `vless://uuid@example.com:443?...`). Если системный
резолвер недоступен, медленный или возвращает нежелательные IP, можно
настроить собственную цепочку через `dns_resolvers` в `config.yaml`.

**Поддерживаемые схемы URL:**

| Схема | Пример | Описание |
|-------|--------|----------|
| `https://host/path` | `https://cloudflare-dns.com/dns-query` | DNS over HTTPS (путь обязателен, например `/dns-query`) |
| `tls://host[:port]` | `tls://1.1.1.1` | DNS over TLS (порт по умолчанию 853) |
| `host[:port]` | `1.1.1.1:53`, `dns.google` | Обычный DNS через UDP/TCP (порт по умолчанию 53) |

**Поведение:**

- Если `dns_resolvers` не указан или пуст — используется `net.DefaultResolver`.
- Порядок в списке определяет приоритет fallback: первый — самый
  предпочтительный; если он вернёт ошибку, запрос идёт ко второму, и так
  далее по цепочке.
- Если все резолверы в цепочке вернули ошибку — возвращается ошибка
  последнего.
- Невалидный URL в `dns_resolvers` — фатальная ошибка конфигурации,
  приложение не стартует.

**Пример с цепочкой DoH/DoT/plain:**

```yaml
dns_resolvers:
  - "https://cloudflare-dns.com/dns-query"   # DoH Cloudflare
  - "https://dns.google/dns-query"           # DoH Google (fallback)
  - "tls://1.1.1.1"                          # DoT Cloudflare
  - "1.1.1.1:53"                             # plain DNS (последний fallback)
```

Под капотом — пакет `internal/resolver` (библиотека
[`github.com/ncruces/go-dns`](https://github.com/ncruces/go-dns)).

### Секции

Каждая секция соответствует URLTest-группе в podkop. Секция объединяет несколько
источников VPN-ссылок разных типов:

```yaml
sections:
  - name: "MULTI_WEST"
    countries:
      - "Lithuania"
      - "Netherlands"
      - "United States"
      - "Sweden"
    sources:
      - type: happ
        urls:
          - "https://hynet.space/s/YOUR_SUBSCRIPTION_ID_1"
          - "https://hynet.space/s/YOUR_SUBSCRIPTION_ID_2"
      - type: plaintext
        urls:
          - "https://raw.githubusercontent.com/user/repo/main/west/vless.txt"
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
| `sources` | array | Список источников VPN-ссылок (минимум 1) |
| `urltest` | object | Опциональное переопределение настроек urltest для секции |

**Поля источника (`source`):**

| Поле | Тип | Описание |
|------|-----|----------|
| `type` | string | Тип источника: `happ` (base64 подписка) или `plaintext` (текст) |
| `urls` | array | Список URL подписок (минимум 1) |

Один и тот же URL может встречаться в нескольких секциях — он будет
обработан только один раз (дедупликация).

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
- Конфиг сохраняется только если outbounds реально изменились

## Логирование

Приложение ведёт структурированные логи в формате key-value. Вы можете указать
директорию для записи лог-файлов через поле `log_dir` в `config.yaml`.

### Настройка логирования

```yaml
# Логи пишутся в файл и одновременно выводятся в stderr
log_dir: "/var/log/vpnconfig"

# Логи только в stderr (значение по умолчанию)
# log_dir: ""
```

### Формат лог-файлов

Каждый запуск создаёт отдельный файл с timestamp в имени:

```
vpnconfig_20260115_143022.log
vpnconfig_20260115_150005.log
vpnconfig_20260115_154511.log
```

### Что логируется

Приложение логирует все ключевые этапы работы:

- **Запуск** — версия, пути к конфигам, количество секций
- **Загрузка ссылок** — сколько ссылок получено из подписки
- **Обработка URL** — каждая ссылка: IP, страна, тип протокола, или причина пропуска
- **Сводка по странам** — сколько серверов в каждой стране
- **Обработка секций** — какие страны разрешены, сколько outbounds найдено/удалено/добавлено
- **Принятие решений** — почему секция пропущена, почему сохранение не требуется
- **Изменения** — сколько outbounds было до и после обновления
- **Бэкапы** — создание бэкапа, удаление старых
- **Результат** — успешное завершение или ошибка

**Пример лога:**

```
time=2026-01-15T14:30:22.123+04:00 level=INFO msg="starting update cycle" cache_path=./cache.json singbox_config=./singbox.json sections_count=2
time=2026-01-15T14:30:22.234+04:00 level=INFO msg="fetching links from source" section=MULTI_WEST type=happ url=https://hynet.space/s/ID_1
time=2026-01-15T14:30:22.456+04:00 level=INFO msg="fetching links from source" section=MULTI_WEST type=plaintext url=https://raw.githubusercontent.com/.../vless.txt
time=2026-01-15T14:30:22.789+04:00 level=INFO msg="parsed url successfully" ip=185.189.46.17 country=Sweden type=vless
time=2026-01-15T14:30:22.790+04:00 level=WARN msg="skipping url: failed to extract IP" url=vmess://... reason="vmess is not supported"
time=2026-01-15T14:30:23.012+04:00 level=INFO msg="url processing summary" total=25 successful=20 skipped=5
time=2026-01-15T14:30:23.013+04:00 level=INFO msg="country summary" country=Netherlands urls=4
time=2026-01-15T14:30:23.100+04:00 level=INFO msg="processing section" section=MULTI_WEST allowed_countries="[Lithuania Netherlands United States Sweden]"
time=2026-01-15T14:30:23.101+04:00 level=INFO msg="building section outbounds" section=MULTI_WEST proxy_count=16
time=2026-01-15T14:30:23.102+04:00 level=INFO msg="removed old section outbounds" section=MULTI_WEST removed_count=18
time=2026-01-15T14:30:23.103+04:00 level=INFO msg="changes detected in sing-box config" old_outbound_count=27 new_outbound_count=27 sections_updated="[MULTI_WEST MULTI_RU]"
time=2026-01-15T14:30:23.104+04:00 level=INFO msg="backup created" path=backups/singbox.json.backup_20260115_143022
time=2026-01-15T14:30:23.105+04:00 level=INFO msg="sing-box config saved" path=./singbox.json
time=2026-01-15T14:30:23.106+04:00 level=INFO msg="update completed" changed=true links_fetched=25 urls_parsed=20 sections_updated="[MULTI_WEST MULTI_RU]"
```

## Использование

```bash
# Запуск
GOPROXY=direct go run ./cmd/updater/main.go

# Сборка
make build

# Сборка для роутера (OpenWRT ARM64)
make build-router

# Тесты
make test

# Проверка качества кода
make check
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
├── profile/
│   ├── profile.go               # Общий интерфейс LinkFetcher
│   ├── happ/
│   │   └── client.go            # Клиент hynet.space (base64)
│   └── plaintext/
│       └── client.go            # Клиент для plain text подписок
├── resolver/
│   └── resolver.go              # Кастомные DNS-резолверы (DoH/DoT/plain) с fallback-цепочкой
├── http/
│   └── client.go                # HTTP-клиент с ротацией User-Agent
├── ipserv/
│   ├── service.go               # Сервис с кешированием
│   ├── fallback.go              # Последовательный fallback между провайдерами
│   ├── cache.go                 # Файловый кеш
│   ├── providers/
│   │   ├── ipapi.go             # ipapi.co
│   │   ├── ipapicom.go          # ip-api.com
│   │   ├── ipwho.go             # ipwho.is
│   │   ├── twoip.go             # api.2ip.me
│   │   ├── ipsb.go              # api.ip.sb
│   │   ├── freegeoip.go         # freegeoip.app
│   │   └── factory.go           # Создание провайдеров по имени
│   └── mmdb/
│       ├── provider.go          # MMDB-провайдер (MaxMind GeoLite2)
│       └── ensure.go            # Скачивание MMDB-базы с автосозданием директорий
├── logger/
│   └── logger.go                # Логирование в файл и stderr
├── singbox/
│   └── config.go                # Манипуляции с конфигурацией sing-box
├── useragent/
│   └── useragent.go             # Генератор User-Agent
├── vpnurl/
│   ├── parser.go                # Парсер VPN-URL
│   ├── vless.go                 # Парсер VLESS
│   ├── trojan.go                # Парсер Trojan
│   └── shadowsocks.go           # Парсер Shadowsocks
└── integration/                 # E2E тесты на mock HTTP/DNS (без внешней сети)
```

### Профили источников

Каждая секция может иметь несколько источников разного типа. За каждый тип
отвечает отдельный пакет в `internal/profile/`:

| Тип | Пакет | Описание |
|-----|-------|----------|
| `happ` | `internal/profile/happ` | Base64-encoded подписка (формат hynet.space). Декодирует ответ, разбивает по строкам. |
| `plaintext` | `internal/profile/plaintext` | Plain text подписка: одна VPN-ссылка на строку. Пустые строки игнорируются. |

Все типы реализуют общий интерфейс `profile.LinkFetcher` с методом
`FetchLinks(ctx, url) ([]string, error)`. `updater` принимает мапу
`map[config.SourceType]LinkFetcher` — для каждого `type` из `sources`
используется соответствующий fetcher.

## Поддерживаемые протоколы

- **VLESS** — с поддержкой Reality/TLS
- **Trojan** — с поддержкой TLS/WebSocket/gRPC
- **Shadowsocks** — с base64-кодированными credentials
- **vmess** — явно не поддерживается и пропускается

## Кеширование

Определение страны по IP кешируется в файл (`cache.json`) для уменьшения количества запросов к внешним API:

- Формат: `{"IP": {"country": "...", "timestamp": "..."}}`
- При превышении `max_cache_size_bytes` кеш очищается
- Кеш дополняется (read → append → write)

Цепочка обработки запроса: `Service (кеш) → Fallback → [Provider1, Provider2, ...]`.

## Бэкапы

Перед каждым обновлением создаётся бэкап конфигурации sing-box:

- Формат имени: `singbox.json.backup_YYYYMMDD_HHMMSS`
- Хранятся в `backup_dir`
- При превышении `max_backups` удаляются самые старые
- Бэкап создаётся только если конфиг реально изменился

## E2E тесты (`internal/integration`)

Сквозные тесты прогоняют **реальный production-код** (`updater.Run`,
`singbox.GenerateSectionOutbounds`, `ipserv.NewCachedIPLookup`, `resolver.Resolver`)
на полностью mock-инфраструктуре — без обращения к внешним сетям.

**Mock-инфраструктура:**

- `mockHTTPServer` — in-process HTTP-сервер, имитирующий hynet.space и все
  6 geo-провайдеров. Диспетчеризация по `X-Original-Host`.
- `dnsServer` — минимальный UDP DNS-сервер по RFC 1035, совместимый
  с `plainResolver` в `internal/resolver`.
- `testEnv` — собирает `*updater.Updater` с реальными компонентами, подменяя
  HTTP-клиент, DNS-резолвер, кеш и файловую систему на временные.

**Покрытие (9 тестов):**

| Тест | Сценарий |
|------|----------|
| `TestE2E_FullCycle` | 21 ссылка (16 west + 4 ru + 1 vmess), 2 секции, бэкап, stale outbounds |
| `TestE2E_NoChange` | Идемпотентность — файл не перезаписывается |
| `TestE2E_VMessSkipped` | vmess пропускается |
| `TestE2E_OutboundsPreserved` | Чужие outbounds сохраняются |
| `TestE2E_RulesetsPreserved` | Rulesets (`local`/`remote`) сохраняются |
| `TestE2E_AllGeoProviders` | Каждый из 6 провайдеров изолированно |
| `TestE2E_GeoProviderFallback` | Fallback при 500 от первых 4 провайдеров |
| `TestE2E_DNSResolution` | Домен резолвится через mock DNS, попадает в секцию |
| `TestMockDNS_Smoke` | Smoke-тест mock DNS |

**Принципы:**

- Никаких секретов в коде — UUID/пароли/ключи нулевые или `TestPublicKey`.
- Никаких внешних сетевых вызовов — всё на `127.0.0.1:<random_port>`.
- Реальный production-код — мокаем только I/O-границу (HTTP, DNS, FS).
- `t.Parallel()` на всех тестах — suite отрабатывает за <2s.

## Дисклеймеры

⚠️ **Этот проект полностью написан нейросетью Kimi K2.5.**

⚠️ **Автор имеет лишь поверхностную экспертизу в sing-box.** Проект создан экспериментальным путём, возможны ошибки в понимании внутренней логики sing-box и podkop.

⚠️ **Это личный проект для домашнего использования.** Не предназначен для production-использования без дополнительного тестирования. Используйте на свой страх и риск.

## Лицензия

MIT License. См. [LICENSE](LICENSE).
