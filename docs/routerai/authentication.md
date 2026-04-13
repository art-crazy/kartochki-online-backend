Аутентификация

# Аутентификация

RouterAI использует API-ключи для аутентификации запросов к API.

## Типы ключей

### API-ключ

Используется для вызовов AI-моделей (chat completions, completions).

**Создание:**

1.  Перейдите в [Настройки → API-ключи](https://routerai.ru/settings/keys)
2.  Нажмите “Создать новый ключ”
3.  Скопируйте и сохраните ключ в безопасном месте

**Использование:**

```
curl -H "Authorization: Bearer YOUR_API_KEY" \
  https://routerai.ru/api/v1/chat/completions
```

Копировать

### Мастер-ключ (Master Key)

Используется для программного управления API-ключами (создание, просмотр, удаление).

**Создание:**

1.  Перейдите в [Настройки → Мастер-ключи](https://routerai.ru/settings/provisioning_keys)
2.  Нажмите “Создать мастер-ключ”
3.  Скопируйте и сохраните ключ в безопасном месте

**Использование:**

```
curl -H "Authorization: Bearer YOUR_MASTER_KEY" \
  https://routerai.ru/api/v1/keys
```

Копировать

## Различия между типами ключей

Характеристика

API-ключ

Мастер-ключ

Назначение

Вызовы AI-моделей

Управление API-ключами

Создание

Через UI или API

Только через UI

Используется для

/chat/completions, /completions

/keys/\*

## Лучшие практики безопасности

*   **Никогда не делитесь своим API-ключом** - Храните его в секрете
*   **Используйте переменные окружения** - Не вставляйте ключи прямо в исходный код
*   **Регулярно обновляйте ключи** - Периодически создавайте новые ключи и удаляйте старые
*   **Отслеживайте использование** - Проверяйте статистику на предмет необычных паттернов
*   **Используйте отдельные ключи** - Создавайте разные ключи для разных приложений

## Примеры использования

### Python

```
import os
from openai import OpenAI

# Используйте переменные окружения
api_key = os.getenv('ROUTERAI_API_KEY')

client = OpenAI(
    api_key=api_key,
    base_url="https://routerai.ru/api/v1"
)
```

Копировать

### JavaScript

```
// Используйте переменные окружения
const client = new OpenAI({
  apiKey: process.env.ROUTERAI_API_KEY,
  baseURL: 'https://routerai.ru/api/v1',
});
```

Копировать

### cURL

```
# Используйте переменные окружения
curl -H "Authorization: Bearer $ROUTERAI_API_KEY" \
  https://routerai.ru/api/v1/chat/completions
```