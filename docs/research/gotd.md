# gotd/td — что проверено для M2 (2026-09-26)

Версия `github.com/gotd/td v0.162.0` (go 1.25). Исходники: `~/go/pkg/mod/github.com/gotd/td@v0.162.0`.

- Клиент: `telegram.NewClient(appID int, appHash string, opt telegram.Options)`; работа — внутри `client.Run(ctx, func(ctx) error)`;
  API — `client.API()` (`*tg.Client`), вход — `client.Auth()` (`*auth.Client`), текущий пользователь — `client.Self(ctx)`.
- Вход по шагам (под экраны): `Auth().SendCode(ctx, phone, auth.SendCodeOptions{}) (tg.AuthSentCodeClass, error)` →
  `Auth().SignIn(ctx, phone, code, codeHash)`; ошибка `auth.ErrPasswordAuthNeeded` → `Auth().Password(ctx, password)`.
  Проверка входа — `Auth().Status(ctx)`.
- FLOOD_WAIT: `tgerr.AsFloodWait(err) (time.Duration, bool)`.
- Хранилище сессии — `session.Storage` (`session/session.go`): `LoadSession(ctx) ([]byte, error)`, `StoreSession(ctx, []byte) error`;
  нет сессии → `session.ErrNotFound`. В клиент — `telegram.Options{SessionStorage: …, NoUpdates: true}`.
- Ошибки RPC: `tgerr.Is(err, "PHONE_CODE_INVALID", …)`; 2FA — `auth.ErrPasswordAuthNeeded`; статус — `auth.Status{Authorized, User}`;
  хэш кода — `(*tg.AuthSentCode).GetPhoneCodeHash()`.
- Диалоги: `query.GetDialogs(api).BatchSize(100).Iter()`; `Next(ctx)`/`Value()`/`Err()`; `Elem{Peer tg.InputPeerClass, Entities peer.Entities}`
  (`github.com/gotd/td/telegram/message/peer`: `Entities.Channel(id)`, `Entities.Chat(id)`). Группы: `*tg.InputPeerChannel` с
  `Channel.GetMegagroup()` и `*tg.InputPeerChat`.
- История: `api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer, OffsetID, Limit, MinID, …})` → `MessagesMessagesClass`,
  `.AsModified()` → `GetMessages()/GetUsers()/GetChats()`. Сообщения — от новых к старым, `id < OffsetID` (0 — с самого нового), `id > MinID`.
- Проверка существования: `api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{Channel: &tg.InputChannel{…}, ID: []tg.InputMessageClass{&tg.InputMessageID{ID}}})`;
  для обычного чата — `api.MessagesGetMessages(ctx, ids)`. Удалённое приходит как `*tg.MessageEmpty`.
- Сообщение `*tg.Message`: `GetID, GetDate, GetMessage, GetFromID (PeerUser/PeerChannel), GetReplyTo (*tg.MessageReplyHeader: GetReplyToMsgID, GetReplyToTopID),
  GetFwdFrom (GetFromName, GetFromID), GetEditDate, GetMedia` (`*tg.MessageMediaPhoto.GetPhoto → *tg.Photo`, `*tg.MessageMediaDocument.GetDocument → *tg.Document`:
  `GetID, GetMimeType, GetSize, GetAttributes` → `*tg.DocumentAttributeFilename.GetFileName`).
