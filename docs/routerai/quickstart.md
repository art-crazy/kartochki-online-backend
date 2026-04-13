Быстрый старт

# Быстрый старт

Это руководство поможет вам начать работу с RouterAI API за несколько минут.

## Шаг 1: Получите API-ключ

1.  Зарегистрируйтесь на [routerai.ru](https://routerai.ru/users/sign_up)
2.  Перейдите в [Настройки → API-ключи](https://routerai.ru/settings/keys)
3.  Нажмите “Создать новый ключ”
4.  Скопируйте и сохраните ключ в безопасном месте

## Шаг 2: Пополните баланс

Перейдите в раздел [Биллинг](https://routerai.ru/settings/billing) и пополните баланс удобным способом:

*   Банковской картой
*   По безналичному расчету

## Шаг 3: Сделайте первый запрос

### cURL

```
curl https://routerai.ru/api/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "openai/gpt-4o",
    "messages": [
      {
        "role": "user",
        "content": "Привет!"
      }
    ]
  }'
```

Копировать

### Python

```
from openai import OpenAI

client = OpenAI(
    api_key="YOUR_API_KEY",
    base_url="https://routerai.ru/api/v1"
)

response = client.chat.completions.create(
    model="openai/gpt-4o",
    messages=[
        {"role": "user", "content": "Привет!"}
    ]
)

print(response.choices[0].message.content)
```

Копировать

### JavaScript

```
import OpenAI from 'openai';

const client = new OpenAI({
  apiKey: 'YOUR_API_KEY',
  baseURL: 'https://routerai.ru/api/v1',
});

async function main() {
  const response = await client.chat.completions.create({
    model: 'openai/gpt-4o',
    messages: [{ role: 'user', content: 'Привет!' }],
  });

  console.log(response.choices[0].message.content);
}

main();
```

Копировать

## Что дальше?

*   Изучите [доступные модели](/models)
*   Настройте [интеграцию с VS Code](/docs/guides/integrations/vscode)
*   Узнайте больше об [аутентификации](/docs/guides/overview/authentication)