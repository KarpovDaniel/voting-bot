package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/mock"
	"voting-bot/tarantool"
)

type MockTarantool struct {
	mock.Mock
}

func (m *MockTarantool) CreatePoll(ctx context.Context, pollID, creatorID, question string, options []string) error {
	args := m.Called(ctx, pollID, creatorID, question, options)
	return args.Error(0)
}

func (m *MockTarantool) GetPoll(ctx context.Context, pollID string) (*tarantool.Poll, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*tarantool.Poll), args.Error(1)
}

func (m *MockTarantool) AddVote(ctx context.Context, pollID, userID, option string) error {
	args := m.Called(ctx, pollID, userID, option)
	return args.Error(0)
}

func (m *MockTarantool) GetResults(ctx context.Context, pollID string) (*tarantool.VoteResult, error) {
	args := m.Called(ctx, pollID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*tarantool.VoteResult), args.Error(1)
}

func (m *MockTarantool) UpdatePollStatus(ctx context.Context, pollID, status string) error {
	args := m.Called(ctx, pollID, status)
	return args.Error(0)
}

func (m *MockTarantool) DeletePoll(ctx context.Context, pollID string) error {
	args := m.Called(ctx, pollID)
	return args.Error(0)
}

func (m *MockTarantool) Close() error {
	return nil
}

type MockMattermostClient struct {
	mock.Mock
}

func (m *MockMattermostClient) CreatePost(ctx context.Context, post *model.Post) (*model.Post, *model.Response, error) {
	args := m.Called(ctx, post)
	return args.Get(0).(*model.Post), args.Get(1).(*model.Response), args.Error(2)
}

func (m *MockMattermostClient) GetMe(ctx context.Context, etag string) (*model.User, *model.Response, error) {
	args := m.Called(ctx, etag)
	return args.Get(0).(*model.User), args.Get(1).(*model.Response), args.Error(2)
}

func TestHandleCreatePoll(t *testing.T) {
	mockTarantool := new(MockTarantool)
	mockMM := new(MockMattermostClient)

	tests := []struct {
		name        string
		args        []string
		setupMocks  func()
		expectError bool
	}{
		{
			name: "successful creation",
			args: []string{"Test question?", "Option1", "Option2"},
			setupMocks: func() {
				mockTarantool.On(
					"CreatePoll",
					context.Background(),
					mock.AnythingOfType("string"),
					"test-user",
					"Test question?",
					[]string{"Option1", "Option2"},
				).Return(nil)

				mockMM.On(
					"CreatePost",
					context.Background(),
					mock.MatchedBy(func(post *model.Post) bool {
						return strings.Contains(post.Message, "Голосование создано! ID: `") &&
							strings.Contains(post.Message, "Test question?") &&
							strings.Contains(post.Message, "1. Option1") &&
							strings.Contains(post.Message, "2. Option2")
					}),
				).Return(&model.Post{}, &model.Response{}, nil)
			},
		},
		{
			name: "insufficient arguments",
			args: []string{"Single argument"},
			setupMocks: func() {
				mockMM.On(
					"CreatePost",
					context.Background(),
					mock.MatchedBy(func(post *model.Post) bool {
						return strings.Contains(post.Message, "Использование: /createpoll")
					}),
				).Return(&model.Post{}, &model.Response{}, nil)
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()

			bot := &Bot{
				Client:          mockMM,
				TarantoolClient: mockTarantool,
				UserID:          "test-user",
			}

			post := &model.Post{
				UserId:    "test-user",
				ChannelId: "test-channel",
				Message:   "/createpoll " + strings.Join(tc.args, " "),
			}

			bot.handleCreatePoll(post, tc.args)

			if tc.expectError {
				mockMM.AssertCalled(t, "CreatePost", context.Background(), mock.Anything)
			} else {
				mockTarantool.AssertExpectations(t)
				mockMM.AssertExpectations(t)
			}

			mockTarantool.ExpectedCalls = nil
			mockMM.ExpectedCalls = nil
		})
	}
}

func TestHandleVote(t *testing.T) {
	mockTarantool := new(MockTarantool)
	mockMM := new(MockMattermostClient)

	poll := &tarantool.Poll{
		PollID:    "test-poll",
		Options:   []string{"A", "B"},
		Status:    "active",
		CreatorID: "test-user",
	}

	tests := []struct {
		name        string
		args        []string
		setupMocks  func()
		expectError bool
	}{
		{
			name: "valid vote",
			args: []string{"test-poll", "1"},
			setupMocks: func() {
				mockTarantool.On("GetPoll", context.Background(), "test-poll").Return(poll, nil)
				mockTarantool.On("AddVote", context.Background(), "test-poll", "voter-user", "1").Return(nil)
				mockMM.On("CreatePost", context.Background(), mock.Anything).Return(&model.Post{}, &model.Response{}, nil)
			},
		},
		{
			name: "invalid poll",
			args: []string{"invalid-poll", "1"},
			setupMocks: func() {
				mockTarantool.On("GetPoll", context.Background(), "invalid-poll").Return(nil, tarantool.ErrNotFound)
				mockMM.On("CreatePost", context.Background(), mock.Anything).Return(&model.Post{}, &model.Response{}, nil)
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()

			bot := &Bot{
				Client:          mockMM,
				TarantoolClient: mockTarantool,
			}

			post := &model.Post{
				UserId:    "voter-user",
				ChannelId: "test-channel",
				Message:   "/vote " + strings.Join(tc.args, " "),
			}

			bot.handleVote(post, tc.args)

			if tc.expectError {
				mockMM.AssertCalled(t, "CreatePost", context.Background(), mock.Anything)
			} else {
				mockTarantool.AssertExpectations(t)
			}

			mockTarantool.ExpectedCalls = nil
			mockMM.ExpectedCalls = nil
		})
	}
}

func TestHandleEndPoll(t *testing.T) {
	mockTarantool := new(MockTarantool)
	mockMM := new(MockMattermostClient)

	poll := &tarantool.Poll{
		PollID:    "test-poll",
		CreatorID: "creator-user",
		Status:    "active",
	}

	tests := []struct {
		name        string
		userID      string
		args        []string
		setupMocks  func()
		expectError bool
	}{
		{
			name:   "creator ends poll",
			userID: "creator-user",
			args:   []string{"test-poll"},
			setupMocks: func() {
				mockTarantool.On("GetPoll", context.Background(), "test-poll").Return(poll, nil)
				mockTarantool.On("UpdatePollStatus", context.Background(), "test-poll", "closed").Return(nil)
				mockMM.On("CreatePost", context.Background(), mock.Anything).Return(&model.Post{}, &model.Response{}, nil)
			},
		},
		{
			name:   "non-creator fails",
			userID: "other-user",
			args:   []string{"test-poll"},
			setupMocks: func() {
				mockTarantool.On("GetPoll", context.Background(), "test-poll").Return(poll, nil)
				mockMM.On("CreatePost", context.Background(), mock.Anything).Return(&model.Post{}, &model.Response{}, nil)
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()

			bot := &Bot{
				Client:          mockMM,
				TarantoolClient: mockTarantool,
				UserID:          tc.userID,
			}

			post := &model.Post{
				UserId:    tc.userID,
				ChannelId: "test-channel",
				Message:   "/endpoll " + strings.Join(tc.args, " "),
			}

			bot.handleEndPoll(post, tc.args)

			if tc.expectError {
				mockMM.AssertCalled(t, "CreatePost", context.Background(), mock.Anything)
			} else {
				mockTarantool.AssertExpectations(t)
			}

			mockTarantool.ExpectedCalls = nil
			mockMM.ExpectedCalls = nil
		})
	}
}

func TestHandleDeletePoll(t *testing.T) {
	mockTarantool := new(MockTarantool)
	mockMM := new(MockMattermostClient)

	poll := &tarantool.Poll{
		PollID:    "test-poll",
		CreatorID: "creator-user",
		Status:    "active",
	}

	tests := []struct {
		name        string
		userID      string
		args        []string
		setupMocks  func()
		expectError bool
	}{
		{
			name:   "creator deletes poll",
			userID: "creator-user",
			args:   []string{"test-poll"},
			setupMocks: func() {
				mockTarantool.On("GetPoll", context.Background(), "test-poll").Return(poll, nil)
				mockTarantool.On("DeletePoll", context.Background(), "test-poll").Return(nil)
				mockMM.On("CreatePost", context.Background(), mock.Anything).Return(&model.Post{}, &model.Response{}, nil)
			},
		},
		{
			name:   "non-creator fails",
			userID: "other-user",
			args:   []string{"test-poll"},
			setupMocks: func() {
				mockTarantool.On("GetPoll", context.Background(), "test-poll").Return(poll, nil)
				mockMM.On("CreatePost", context.Background(), mock.Anything).Return(&model.Post{}, &model.Response{}, nil)
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()

			bot := &Bot{
				Client:          mockMM,
				TarantoolClient: mockTarantool,
				UserID:          tc.userID,
			}

			post := &model.Post{
				UserId:    tc.userID,
				ChannelId: "test-channel",
				Message:   "/deletepoll " + strings.Join(tc.args, " "),
			}

			bot.handleDeletePoll(post, tc.args)

			if tc.expectError {
				mockMM.AssertCalled(t, "CreatePost", context.Background(), mock.Anything)
			} else {
				mockTarantool.AssertExpectations(t)
			}

			mockTarantool.ExpectedCalls = nil
			mockMM.ExpectedCalls = nil
		})
	}
}
