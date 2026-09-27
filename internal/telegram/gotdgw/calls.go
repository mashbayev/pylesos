// Файл: internal/telegram/gotdgw/calls.go
package gotdgw

import (
	"context"
	"errors"
	"fmt"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/tg"

	"pylesos/internal/telegram/domain"
)

func (g *Gateway) Account(ctx context.Context) (domain.Account, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return domain.Account{}, err
	}
	st, err := c.Auth().Status(ctx)
	if err := mapErr(err); errors.Is(err, domain.ErrNotAuthorized) {
		return domain.Account{}, nil
	} else if err != nil {
		return domain.Account{}, err
	}
	if !st.Authorized {
		return domain.Account{}, nil
	}
	return domain.Account{Authorized: true, Name: userName(st.User)}, nil
}

func (g *Gateway) SendCode(ctx context.Context, phone string) (string, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return "", err
	}
	sent, err := c.Auth().SendCode(ctx, phone, auth.SendCodeOptions{})
	if err != nil {
		return "", mapErr(err)
	}
	code, ok := sent.(*tg.AuthSentCode)
	if !ok {
		return "", fmt.Errorf("неожиданный ответ на запрос кода %T", sent)
	}
	return code.GetPhoneCodeHash(), nil
}

func (g *Gateway) SignIn(ctx context.Context, phone, code, codeHash string) error {
	c, err := g.conn(ctx)
	if err != nil {
		return err
	}
	_, err = c.Auth().SignIn(ctx, phone, code, codeHash)
	return mapErr(err)
}

func (g *Gateway) Password(ctx context.Context, password string) error {
	c, err := g.conn(ctx)
	if err != nil {
		return err
	}
	_, err = c.Auth().Password(ctx, password)
	return mapErr(err)
}

func (g *Gateway) Groups(ctx context.Context) ([]domain.Group, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}
	iter := query.GetDialogs(c.API()).BatchSize(100).Iter()
	groups := []domain.Group{}
	for iter.Next(ctx) {
		if gr, ok := groupOf(iter.Value()); ok {
			groups = append(groups, gr)
		}
	}
	if err := iter.Err(); err != nil {
		return nil, mapErr(err)
	}
	return groups, nil
}

func (g *Gateway) History(ctx context.Context, p domain.Peer, offsetID, minID, limit int) (domain.Page, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return domain.Page{}, err
	}
	res, err := c.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer: inputPeer(p), OffsetID: offsetID, MinID: minID, Limit: limit})
	if err != nil {
		return domain.Page{}, mapErr(err)
	}
	return messagesFrom(res)
}

func (g *Gateway) Alive(ctx context.Context, p domain.Peer, ids []int) (map[int]bool, error) {
	c, err := g.conn(ctx)
	if err != nil {
		return nil, err
	}
	inputs := make([]tg.InputMessageClass, 0, len(ids))
	for _, id := range ids {
		inputs = append(inputs, &tg.InputMessageID{ID: id})
	}
	var res tg.MessagesMessagesClass
	if p.Type == domain.PeerChat {
		res, err = c.API().MessagesGetMessages(ctx, inputs)
	} else {
		res, err = c.API().ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: p.ID, AccessHash: p.AccessHash}, ID: inputs})
	}
	if err != nil {
		return nil, mapErr(err)
	}
	return aliveFrom(res)
}
