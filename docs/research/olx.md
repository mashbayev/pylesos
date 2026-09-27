# olx.kz — как устроен доступ к объявлениям

Проверено 2026-09-26.

## Прямой HTTP-клиент не работает

`curl` на `https://www.olx.kz/api/v1/offers/...` и даже на `robots.txt` → **403 от CloudFront** («Request blocked»).
Сайт закрыт защитой от ботов для запросов не из браузера. Обход этой защиты (подмена отпечатков, прокси и т.п.)
не рассматривается.

## В обычном браузере работает

Поиск: `https://www.olx.kz/list/q-<запрос>/` — 52 карточки на странице, до 25 страниц.

Внутренний JSON API сайта отвечает из контекста открытой страницы olx.kz (тот же источник, те же cookies):
`GET /api/v1/offers/?offset=N&limit=M&query=<запрос>` → 200.

- `metadata.total_elements` — не больше **1000** на запрос (для «сотен объявлений» хватает; больше — сужать фильтрами).
- Поля объявления: `id, url, title, description, created_time, last_refresh_time, valid_to_time, params
  (параметры категории, напр. тип аренды), key_params, business, user (name, company_name, seller_type, created…),
  contact (name, phone: bool — есть ли телефон, chat), location, map, photos, category, promotion, offer_type`.
- Телефона в ответе нет — только признак `contact.phone = true`. Сам номер отдаётся отдельным запросом
  после нажатия «Показать телефон», которому нужен вход в аккаунт OLX (П11).
