// Файл: internal/telegram/gotdgw/media.go
package gotdgw

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"

	"pylesos/internal/telegram/domain"
)

// Media скачивает вложения сообщения. Сообщение запрашивается заново: у файла свежая ссылка доступа
// (file_reference устаревает). Файлы держатся в памяти и на диск не пишутся (П12).
func (g *Gateway) Media(ctx context.Context, p domain.Peer, msgID int) ([]domain.File, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}
	msg, err := g.fetchMessage(ctx, c.API(), p, msgID)
	if err != nil || msg == nil {
		return nil, err
	}
	loc, name, ok := location(msg)
	if !ok {
		return nil, nil
	}
	var buf bytes.Buffer
	if _, err := downloader.NewDownloader().Download(c.API(), loc).Stream(ctx, &buf); err != nil {
		return nil, mapErr(err)
	}
	return []domain.File{{Name: name, Data: buf.Bytes()}}, nil
}

// fetchMessage — сообщение по номеру; nil — удалено.
func (g *Gateway) fetchMessage(ctx context.Context, api *tg.Client, p domain.Peer, msgID int) (*tg.Message, error) {
	ids := []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}}
	var res tg.MessagesMessagesClass
	var err error
	if p.Type == domain.PeerChat {
		res, err = api.MessagesGetMessages(ctx, ids)
	} else {
		res, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: p.ID, AccessHash: p.AccessHash}, ID: ids})
	}
	if err != nil {
		return nil, mapErr(err)
	}
	mod, ok := res.AsModified()
	if !ok {
		return nil, fmt.Errorf("неожиданный ответ %T", res)
	}
	for _, m := range mod.GetMessages() {
		if msg, ok := m.(*tg.Message); ok && msg.GetID() == msgID {
			return msg, nil
		}
	}
	return nil, nil
}

// location — откуда скачивать вложение и под каким именем; false — вложения-файла нет.
func location(m *tg.Message) (tg.InputFileLocationClass, string, bool) {
	media, ok := m.GetMedia()
	if !ok {
		return nil, "", false
	}
	switch v := media.(type) {
	case *tg.MessageMediaPhoto:
		if p, ok := v.GetPhoto(); ok {
			if photo, ok := p.(*tg.Photo); ok {
				return photo.AsInputPhotoFileLocation(largestSize(photo)), "photo.jpg", true
			}
		}
	case *tg.MessageMediaDocument:
		if d, ok := v.GetDocument(); ok {
			if doc, ok := d.(*tg.Document); ok {
				return doc.AsInputDocumentFileLocation(""), docName(doc), true
			}
		}
	}
	return nil, "", false
}

// largestSize — тип самого большого размера фото.
func largestSize(p *tg.Photo) string {
	best, area := "", 0
	for _, s := range p.GetSizes() {
		var t string
		var w, h int
		switch v := s.(type) {
		case *tg.PhotoSize:
			t, w, h = v.Type, v.W, v.H
		case *tg.PhotoSizeProgressive:
			t, w, h = v.Type, v.W, v.H
		default:
			continue
		}
		if w*h > area {
			best, area = t, w*h
		}
	}
	return best
}

// docName — имя файла документа; без имени — «document» с расширением по типу.
func docName(d *tg.Document) string {
	if name := documentOf(d).FileName; name != "" {
		return name
	}
	switch d.GetMimeType() {
	case "application/pdf":
		return "document.pdf"
	case "image/png":
		return "document.png"
	case "image/jpeg":
		return "document.jpg"
	case "image/tiff":
		return "document.tif"
	}
	return "document"
}
