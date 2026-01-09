package telegram

import (
	"context"
	"fmt"
	"tg-blobsync/internal/domain"

	"github.com/gotd/td/tg"
)

// ListGroups returns a list of Supergroups.
func (t *TelegramClient) ListGroups(ctx context.Context) ([]domain.Group, error) {
	dialogs, err := t.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		Limit:      100,
		OffsetPeer: &tg.InputPeerEmpty{},
	})
	if err != nil {
		return nil, err
	}

	var groups []domain.Group
	var chats []tg.ChatClass

	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	}

	for _, chat := range chats {
		switch c := chat.(type) {
		case *tg.Channel:
			if c.Megagroup {
				t.setAccessHash(c.ID, c.AccessHash)
				groups = append(groups, domain.Group{
					ID:    c.ID,
					Title: c.Title,
				})
			}
		}
	}

	return groups, nil
}

// ResolveGroup ensures the AccessHash for the given groupID is cached.
func (t *TelegramClient) ResolveGroup(ctx context.Context, groupID int64) error {
	if _, ok := t.getAccessHash(groupID); ok {
		return nil
	}
	_, err := t.ListGroups(ctx)
	if err != nil {
		return err
	}
	if _, ok := t.getAccessHash(groupID); ok {
		return nil
	}
	return fmt.Errorf("group %d not found in recent dialogs", groupID)
}

// ListTopics returns a list of Forum Topics in a Supergroup.
func (t *TelegramClient) ListTopics(ctx context.Context, groupID int64) ([]domain.Topic, error) {
	accessHash, _ := t.getAccessHash(groupID)
	inputPeer := &tg.InputPeerChannel{
		ChannelID:  groupID,
		AccessHash: accessHash,
	}

	res, err := t.api.MessagesGetForumTopics(ctx, &tg.MessagesGetForumTopicsRequest{
		Peer:  inputPeer,
		Limit: 100,
	})
	if err != nil {
		return nil, err
	}

	var topics []domain.Topic
	for _, topic := range res.Topics {
		switch t := topic.(type) {
		case *tg.ForumTopic:
			topics = append(topics, domain.Topic{
				ID:    int64(t.ID),
				Title: t.Title,
			})
		}
	}

	return topics, nil
}
