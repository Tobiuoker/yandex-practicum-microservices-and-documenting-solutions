# Project_template

Это шаблон для решения проектной работы. Структура этого файла повторяет структуру заданий. Заполняйте его по мере работы над решением.

# Задание 1. Анализ и планирование

<aside>

Чтобы составить документ с описанием текущей архитектуры приложения, можно часть информации взять из описания компании и условия задания. Это нормально.

</aside

### 1. Описание функциональности монолитного приложения

**Управление отоплением:**

- Пользователи могут создавать, обновлять и удалять сенсоры. Также возможно изменение значения самого датчика, то есть реализовано управление температурой
- Система поддерживает локальное фиксирование в БД данных по датчикам

**Мониторинг температуры:**

- Пользователи могут получить информацию как по конкретному датчику, так и по всем сразу
- Система поддерживает автоматическое синхронное обновление данных датчика температуры из удалённого АПИ при запросе информации о нём

### 2. Анализ архитектуры монолитного приложения

Язык программирования: Go
База данных: PostgreSQL
Архитектура: Монолитная, но есть участки кода поделены на слои (насколько это возможно)
- В слое handle находится транспортный слой, то есть ручки, которые доступны для вызовы извне
- Папка service содержит слой для взаимодействия с внешним АПИ - в нашем случае АПИ температуры
- DB можно назвать слоем доступа к данным, т.к. реализует работу с БД Postgres
- И папка models хранит как доменную модель, так и модели для запросов во внешнюю систему
Взаимодействие: все ручки сервиса вызываются конкурентно (благодаря горутинам в Go), но вот запросы во внешнее апи (например в ручке по получению инфы по датчикам) делаются синхронно по очереди для каждого датчика
Масштабируемость: Ограничена, так как монолит сложно масштабировать по частям из-за сильной связности его контекстов (мониторинг и управление датчиками)
Развертывание: Требует остановки всего приложения.

### 3. Определение доменов и границы контекстов

Device Registry - реестр датчиков, их статус, тип, расположение и тд
Telemetry - история измерений по показаниям
Device Connectivity - подключение датчиков, MQTT-взаимодействие, доставка команд
Scenarios - пользовательские сценарии, запускаемые по расписанию, состояниям или показаниям telemetry
API Gateway - единая публичная точка входа и проверка доступа

### **4. Проблемы монолитного решения**

- Монолит знает про все: про внешнее АПИ, про хранение и получение температуры
- Пока что поддержан только датчик с температурой, а дальнейшее их расширение потребует больших правок в коде
- В ручках получения инфы о датчиках монолит их обходит и делает блокирующий запрос в сторонний сервис
- Нельзя посмотреть историю изменения какого-либо датчика (будет сложно построить красивые дэшборды с детализацией)


### 5. Визуализация контекста системы — диаграмма С4

Добавьте сюда диаграмму контекста в модели C4.

[Диаграмма монолитного контекста](schemas/monolith_context_diagram.puml)


# Задание 2. Проектирование микросервисной архитектуры

В этом задании вам нужно предоставить только диаграммы в модели C4. Мы не просим вас отдельно описывать получившиеся микросервисы и то, как вы определили взаимодействия между компонентами To-Be системы. Если вы правильно подготовите диаграммы C4, они и так это покажут.

**Диаграмма контейнеров (Containers)**

[Контейнерная диаграмма](schemas/container_diagram.puml)

**Диаграмма компонентов (Components)**

[Device Registry Service](schemas/device_registry_component_diagram.puml);
[Device Connection Service](schemas/device_connection_service_component_diagram.puml);
[Telemetry Service](schemas/telemetry_service_component_diagram.puml);
[Scenarios Service](schemas/scenarios_component_diagram.puml).

**Диаграмма кода (Code)**

Критичный сценарий создания и валидации команды устройству описан в [тут](schemas/device_registry_command_creation_and_validation_code_diagram.puml).

# Задание 3. Разработка ER-диаграммы

[ER-диаграмма](schemas/er_diagram.puml)

ER-модель отражает ключевые сущности будущей системы:

- User
- House
- Device
- DeviceCommand
- TelemetryData
- Scenario
- ScenarioTrigger
- ScenarioAction

Основные связи:

- пользователь владеет домами
- дом содержит устройства
- устройство генерирует telemetry
- устройство получает команды
- сценарий принадлежит дому
- сценарий имеет triggers и actions
- action может быть направлен на конкретное устройство

# Задание 4. Создание и документирование API

В решении используются два типа API

### REST API

REST API используется для синхронных взаимодействий, когда вызывающему сервису нужен немедленный ответ.

Минимальный набор endpoint'ов:

`GET /internal/v1/devices/{deviceId}` - получить информацию об устройстве
`PATCH /internal/v1/devices/{deviceId}/connection-status` - обновить online/offline статус устройства
`POST /internal/v1/devices/{deviceId}/commands` - создать команду устройству
`GET /internal/v1/telemetry` - получить исторические метрики
`POST /internal/v1/scenarios` - Создать пользовательский сценарий

OpenAPI документация находится [`тут`](schemas/rest_api.yaml).

### AsyncAPI

AsyncAPI используется для асинхронного взаимодействия через Kafka, когда producer не ждёт немедленного ответа:

| Topic | Producer | Consumers | Назначение |
|---|---|---|---|
| `device.telemetry` | Device Connection Service | Telemetry Service, Scenarios Service | Новое измерение датчика |
| `device.state` | Device Connection Service | Device Registry Service, Scenarios Service | Изменение состояния устройства |
| `device.commands` | Device Registry Service | Device Connection Service | Команда на доставку устройству |
| `device.command_status` | Device Connection Service | Device Registry Service | Статус доставки или выполнения команды |

AsyncAPI документация находится в [`тут`](schemas/async_api.yaml).


---

# Задание 5. Работа с docker и docker-compose

Все сервисы запускаются через [`apps/docker-compose.yml`](apps/docker-compose.yml).

Состав окружения:

| Сервис | Порт | Назначение |
|---|---:|---|
| `app` | 8080 | Go-монолит smart_home |
| `temperature-api` | 8081 | Внешний API температуры |
| `device-service` | 8082 | Новый микросервис реестра устройств |
| `telemetry-service` | 8083 | Новый микросервис телеметрии |
| `postgres` | internal | PostgreSQL монолита |
| `device-postgres` | internal | PostgreSQL device-service |
| `telemetry-postgres` | internal | PostgreSQL telemetry-service |

Запуск из директории [`apps`](apps):

```bash
docker compose up --build
```

Проверка healthcheck монолита:

```bash
curl http://localhost:8080/health
```


# **Задание 6. Разработка MVP**

Было добавлено два микросервиса

1. [Device Service](apps/device-service) — хранит синхронизированные устройства, соответствующие legacy sensors монолита.
2. [Telemetry Service](apps/telemetry-service) — хранит историю telemetry, которую монолит отправляет при чтении или обновлении температуры.

## Device Service

Основные ручки:

| Endpoint | Назначение |
|---|---|
| `POST /devices` | Создать или обновить устройство по `legacy_sensor_id` |
| `GET /devices` | Получить список устройств |
| `GET /devices/by-legacy-sensor/{sensorId}` | Получить устройство по id legacy sensor |
| `PUT /devices/by-legacy-sensor/{sensorId}` | Обновить устройство |
| `DELETE /devices/by-legacy-sensor/{sensorId}` | Пометить устройство удалённым |

Device Service использует отдельную БД `devices`

## Telemetry Service

Основные ручки:

| Endpoint | Назначение |
|---|---|
| `POST /telemetry` | Сохранить новое измерение |
| `GET /telemetry` | Получить историю telemetry |
| `GET /telemetry/latest` | Получить последнее измерение sensor |

Для Telemetry Service также была добавлена отдельная БД `telemetry`

## Интеграция монолита с микросервисами

Монолит получает адреса микросервисов через переменные окружения. Если `ENABLE_MICROSERVICES_SYNC` выставлен в false, то новые сервисы вызываться не будут

- `DEVICE_SERVICE_URL=http://device-service:8082`;
- `TELEMETRY_SERVICE_URL=http://telemetry-service:8083`;
- `ENABLE_MICROSERVICES_SYNC=true`.

Клиенты микросервисов:

- [`Для device service`](apps/smart_home/services/device_client.go);
- [`Для telemetry service`](apps/smart_home/services/telemetry_client.go).

### Интеграции с монолитом

Монолит пишет данные в новые сервисы. Сделал вызов новых микросервисов в отдельных горутинах, чтобы не портить тайминги монолита

| Действие в монолите | Интеграция |
|---|---|
| `Create Sensor` | вызывает `device-service POST /devices` |
| `Update Sensor` | вызывает `device-service PUT /devices/by-legacy-sensor/{id}` |
| `Delete Sensor` | вызывает `device-service DELETE /devices/by-legacy-sensor/{id}` |
| `Get All Sensors` | получает температуру и пишет telemetry |
| `Get Sensor by ID` | получает температуру и пишет telemetry |
| `Update Sensor Value` | пишет telemetry |



Монолит также умеет читать данные из новых сервисов.

Добавлены демонстрационные ручки монолита:

| Endpoint монолита | Что делает |
|---|---|
| `GET /api/v1/sensors/{id}/device` | Читает device из device-service |
| `GET /api/v1/sensors/{id}/telemetry` | Читает историю telemetry из telemetry-service |
| `GET /api/v1/sensors/{id}/telemetry/latest` | Читает последнее telemetry-измерение |
| `GET /api/v1/sensors/{id}/microservices-summary` | Возвращает sensor из монолита + device + latest telemetry |


## Postman-сценарий проверки задания 6

[Коллекция](apps/smarthome-api.postman_collection.json).

Можно проверить так:
1. `Create Sensor`;
2. `Get Sensor by ID` — создаёт telemetry через temperature-api
3. `Update Sensor Value` — дополнительно пишет telemetry
4. `Get Sensor Device` — монолит читает device-service
5. `Get Sensor Telemetry History` — монолит читает telemetry-service
6. `Get Sensor Latest Telemetry` — монолит читает latest telemetry
7. `Get Sensor Microservices Summary` — монолит показывает данные из всех источников одним ответом
