package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"voting-bot/tarantool"
)

type MattermostClient interface {
	CreatePost(ctx context.Context, post *model.Post) (*model.Post, *model.Response, error)
	GetMe(ctx context.Context, etag string) (*model.User, *model.Response, error)
}

type Bot struct {
	Client          MattermostClient
	WebSocket       *model.WebSocketClient
	TarantoolClient tarantool.Client
	UserID          string
}

func NewBot(serverURL, token string, tc tarantool.Client) (*Bot, error) {
	client := model.NewAPIv4Client(serverURL)
	client.SetToken(token)

	user, _, err := client.GetMe(context.Background(), "")
	if err != nil {
		return nil, err
	}

	ws, err := model.NewWebSocketClient4(serverURL, token)
	if err != nil {
		return nil, err
	}

	return &Bot{
		Client:          client,
		WebSocket:       ws,
		TarantoolClient: tc,
		UserID:          user.Id,
	}, nil
}

func (b *Bot) Listen() {
	b.WebSocket.Listen()
	go func() {
		for event := range b.WebSocket.EventChannel {
			if event.EventType() == model.WebsocketEventPosted {
				b.handleMessageEvent(event)
			}
		}
	}()
}

func (b *Bot) handleMessageEvent(event *model.WebSocketEvent) {
	postData := event.GetData()["post"].(string)
	var post *model.Post
	if err := json.Unmarshal([]byte(postData), &post); err != nil {
		log.Printf("Error unmarshaling post: %v", err)
		return
	}

	if post.UserId == b.UserID {
		return
	}

	message := strings.TrimSpace(post.Message)
	if !strings.HasPrefix(message, "/") {
		return
	}

	parts := strings.Fields(message)
	command := parts[0]
	args := parts[1:]

	switch command {
	case "/createpoll":
		b.handleCreatePoll(post, args)
	case "/vote":
		b.handleVote(post, args)
	case "/results":
		b.handleResults(post, args)
	case "/endpoll":
		b.handleEndPoll(post, args)
	case "/deletepoll":
		b.handleDeletePoll(post, args)
	}
}

func (b *Bot) handleCreatePoll(post *model.Post, args []string) {
	if len(args) < 2 {
		b.sendReply(post.ChannelId, "Использование: /createpoll \"Вопрос?\" \"Вариант1\" \"Вариант2\" ...")
		return
	}

	question := args[0]
	options := args[1:]
	pollID := model.NewId()

	err := b.TarantoolClient.CreatePoll(context.Background(), pollID, post.UserId, question, options)
	if err != nil {
		log.Printf("Ошибка создания голосования: %v", err)
		b.sendReply(post.ChannelId, "Не удалось создать голосование")
		return
	}

	response := fmt.Sprintf("Голосование создано! ID: `%s`\n**Вопрос**: %s\n**Варианты**:\n", pollID, question)
	for i, opt := range options {
		response += fmt.Sprintf("%d. %s\n", i+1, opt)
	}
	b.sendReply(post.ChannelId, response)
}

func (b *Bot) handleVote(post *model.Post, args []string) {
	if len(args) != 2 {
		b.sendReply(post.ChannelId, "Использование: /vote ID_ГОЛОСОВАНИЯ НОМЕР_ВАРИАНТА")
		return
	}

	pollID := args[0]
	option := args[1]

	poll, err := b.TarantoolClient.GetPoll(context.Background(), pollID)
	if err != nil || poll == nil {
		b.sendReply(post.ChannelId, "Голосование не найдено")
		return
	}

	if optionNum, err := strconv.Atoi(option); err != nil || optionNum < 1 || optionNum > len(poll.Options) {
		b.sendReply(post.ChannelId, "Неверный номер варианта")
		return
	}

	err = b.TarantoolClient.AddVote(context.Background(), pollID, post.UserId, option)
	if err != nil {
		log.Printf("Ошибка голосования: %v", err)
		b.sendReply(post.ChannelId, "Не удалось сохранить ваш голос")
		return
	}

	b.sendReply(post.ChannelId, "Ваш голос учтён!")
}

func (b *Bot) handleResults(post *model.Post, args []string) {
	if len(args) != 1 {
		b.sendReply(post.ChannelId, "Использование: /results ID_ГОЛОСОВАНИЯ")
		return
	}

	pollID := args[0]

	results, err := b.TarantoolClient.GetResults(context.Background(), pollID)
	if err != nil || results == nil {
		b.sendReply(post.ChannelId, "Голосование не найдено")
		return
	}

	response := fmt.Sprintf("**Результаты голосования**: %s\n", results.Question)
	for i, opt := range results.Options {
		response += fmt.Sprintf("%d. %s - %d голосов\n", i+1, opt, results.Votes[i])
	}
	response += fmt.Sprintf("\nВсего голосов: %d", results.Total)
	b.sendReply(post.ChannelId, response)
}

func (b *Bot) handleEndPoll(post *model.Post, args []string) {
	if len(args) != 1 {
		b.sendReply(post.ChannelId, "Использование: /endpoll ID_ГОЛОСОВАНИЯ")
		return
	}

	pollID := args[0]

	poll, err := b.TarantoolClient.GetPoll(context.Background(), pollID)
	if err != nil || poll == nil {
		b.sendReply(post.ChannelId, "Голосование не найдено")
		return
	}

	if poll.CreatorID != post.UserId {
		b.sendReply(post.ChannelId, "Только создатель может завершить голосование")
		return
	}

	err = b.TarantoolClient.UpdatePollStatus(context.Background(), pollID, "closed")
	if err != nil {
		log.Printf("Ошибка завершения голосования: %v", err)
		b.sendReply(post.ChannelId, "Не удалось завершить голосование")
		return
	}

	b.sendReply(post.ChannelId, "Голосование завершено!")
}

func (b *Bot) handleDeletePoll(post *model.Post, args []string) {
	if len(args) != 1 {
		b.sendReply(post.ChannelId, "Использование: /deletepoll ID_ГОЛОСОВАНИЯ")
		return
	}

	pollID := args[0]

	poll, err := b.TarantoolClient.GetPoll(context.Background(), pollID)
	if err != nil || poll == nil {
		b.sendReply(post.ChannelId, "Голосование не найдено")
		return
	}

	if poll.CreatorID != post.UserId {
		b.sendReply(post.ChannelId, "Только создатель может удалить голосование")
		return
	}

	err = b.TarantoolClient.DeletePoll(context.Background(), pollID)
	if err != nil {
		log.Printf("Ошибка удаления голосования: %v", err)
		b.sendReply(post.ChannelId, "Не удалось удалить голосование")
		return
	}

	b.sendReply(post.ChannelId, "Голосование удалено!")
}

func (b *Bot) sendReply(channelId, message string) {
	post := &model.Post{
		ChannelId: channelId,
		Message:   message,
	}

	if _, _, err := b.Client.CreatePost(context.Background(), post); err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	}
}
