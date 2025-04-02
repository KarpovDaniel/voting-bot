package tarantool

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/tarantool/go-tarantool"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalidOption = errors.New("invalid option")
)

type Client interface {
	CreatePoll(ctx context.Context, pollID, creatorID, question string, options []string) error
	GetPoll(ctx context.Context, pollID string) (*Poll, error)
	AddVote(ctx context.Context, pollID, userID, option string) error
	GetResults(ctx context.Context, pollID string) (*VoteResult, error)
	UpdatePollStatus(ctx context.Context, pollID, status string) error
	DeletePoll(ctx context.Context, pollID string) error
	Close() error
}

type TarantoolClient struct {
	conn *tarantool.Connection
}

type Poll struct {
	PollID    string   `msgpack:"poll_id"`
	CreatorID string   `msgpack:"creator_id"`
	Question  string   `msgpack:"question"`
	Options   []string `msgpack:"options"`
	CreatedAt int64    `msgpack:"created_at"`
	Status    string   `msgpack:"status"`
}

type VoteResult struct {
	Question string
	Options  []string
	Votes    []int
	Total    int
}

func NewTarantoolClient(address, user, password string) (*TarantoolClient, error) {
	opts := tarantool.Opts{
		User:          user,
		Pass:          password,
		Timeout:       10 * time.Second,
		Reconnect:     5 * time.Second,
		MaxReconnects: 5,
	}

	conn, err := tarantool.Connect(address, opts)
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}

	if _, err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	return &TarantoolClient{conn: conn}, nil
}

func (tc *TarantoolClient) CreatePoll(pollID, creatorID, question string, options []string) error {
	_, err := tc.conn.Insert("polls", []interface{}{
		pollID,
		creatorID,
		question,
		options,
		"active",
	})
	return err
}

func (tc *TarantoolClient) GetPoll(pollID string) (*Poll, error) {
	resp, err := tc.conn.Select("polls", "primary", 0, 1, tarantool.IterEq, []interface{}{pollID})
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, ErrNotFound
	}

	data := resp.Data[0].([]interface{})
	return &Poll{
		PollID:    data[0].(string),
		CreatorID: data[1].(string),
		Question:  data[2].(string),
		Options:   convertToStringSlice(data[3].([]interface{})),
		Status:    data[4].(string),
	}, nil
}

func (tc *TarantoolClient) AddVote(pollID, userID, option string) error {
	poll, err := tc.GetPoll(pollID)
	if err != nil {
		return err
	}

	optionNum, err := strconv.Atoi(option)
	if err != nil || optionNum < 1 || optionNum > len(poll.Options) {
		return ErrInvalidOption
	}

	_, err = tc.conn.Replace("votes", []interface{}{
		pollID,
		userID,
		option,
	})
	return err
}

func (tc *TarantoolClient) GetResults(pollID string) (*VoteResult, error) {
	poll, err := tc.GetPoll(pollID)
	if err != nil {
		return nil, err
	}

	resp, err := tc.conn.Call("box.space.votes.index.poll_idx:select", []interface{}{pollID})
	if err != nil {
		return nil, err
	}

	votes := make(map[string]int)
	for _, item := range resp.Data {
		data := item.([]interface{})
		option := data[2].(string)
		votes[option]++
	}

	result := &VoteResult{
		Question: poll.Question,
		Options:  poll.Options,
		Votes:    make([]int, len(poll.Options)),
		Total:    0,
	}

	for i := range poll.Options {
		result.Votes[i] = votes[fmt.Sprint(i+1)]
		result.Total += result.Votes[i]
	}

	return result, nil
}

func (tc *TarantoolClient) UpdatePollStatus(pollID, status string) error {
	_, err := tc.conn.Update("polls", "primary", []interface{}{pollID}, []interface{}{
		[]interface{}{"=", 4, status},
	})
	return err
}

func (tc *TarantoolClient) DeletePoll(pollID string) error {
	if _, err := tc.conn.Delete("polls", "primary", []interface{}{pollID}); err != nil {
		return err
	}

	resp, err := tc.conn.Select("votes", "poll_idx", 0, 0, tarantool.IterEq, []interface{}{pollID})
	if err != nil {
		return err
	}

	// Удалить каждый голос по первичному ключу
	for _, tuple := range resp.Tuples() {
		_, err := tc.conn.Delete("votes", "primary", []interface{}{tuple[0], tuple[1]}) // poll_id, user_id
		if err != nil {
			return err
		}
	}
	return err
}

func (tc *TarantoolClient) Close() error {
	return tc.conn.Close()
}

func convertToStringSlice(in []interface{}) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = v.(string)
	}
	return out
}
