package tarantool

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tarantool/go-tarantool"
)

func TestTarantoolClient(t *testing.T) {
	// Настройка подключения
	client, err := NewTarantoolClient("localhost:3301", "test", "test")
	require.NoError(t, err, "Failed to connect to Tarantool")
	defer client.Close()

	// Генерация уникальных данных для теста
	pollID := "test_poll_" + uuid.New().String()
	userID := "test_user_" + uuid.New().String()
	question := "Test Question?"
	options := []string{"Option1", "Option2"}

	ctx := context.Background()

	t.Run("Create and Get Poll", func(t *testing.T) {
		err := client.CreatePoll(ctx, pollID, userID, question, options)
		assert.NoError(t, err)

		poll, err := client.GetPoll(ctx, pollID)
		require.NoError(t, err)
		require.NotNil(t, poll)

		assert.Equal(t, pollID, poll.PollID)
		assert.Equal(t, userID, poll.CreatorID)
		assert.Equal(t, question, poll.Question)
		assert.Equal(t, options, poll.Options)
		assert.Equal(t, "active", poll.Status)
	})

	t.Run("Vote Handling", func(t *testing.T) {
		// Голосование первого пользователя
		err := client.AddVote(ctx, pollID, "user1", "1")
		assert.NoError(t, err)

		// Голосование второго пользователя
		err = client.AddVote(ctx, pollID, "user2", "2")
		assert.NoError(t, err)

		// Проверка результатов
		results, err := client.GetResults(ctx, pollID)
		require.NoError(t, err)
		require.NotNil(t, results)

		assert.Equal(t, question, results.Question)
		assert.Equal(t, options, results.Options)
		assert.Equal(t, []int{1, 1}, results.Votes)
		assert.Equal(t, 2, results.Total)
	})

	t.Run("Update Poll Status", func(t *testing.T) {
		err := client.UpdatePollStatus(ctx, pollID, "closed")
		assert.NoError(t, err)

		poll, err := client.GetPoll(ctx, pollID)
		require.NoError(t, err)
		require.NotNil(t, poll)

		assert.Equal(t, "closed", poll.Status)
	})

	t.Run("Delete Poll", func(t *testing.T) {
		err := client.DeletePoll(ctx, pollID)
		assert.NoError(t, err)

		_, err = client.GetPoll(ctx, pollID)
		assert.Error(t, err)
	})

	t.Run("Negative Cases", func(t *testing.T) {
		t.Run("Non-existent Poll", func(t *testing.T) {
			_, err := client.GetPoll(ctx, "non_existent_poll")
			assert.Error(t, err)
		})

		t.Run("Invalid Option", func(t *testing.T) {
			err := client.AddVote(ctx, pollID, "user3", "3")
			assert.Error(t, err)
		})
	})
}

func TestMain(m *testing.M) {
	// Очистка тестовых данных перед запуском
	cleanupTestData()
	m.Run()
	cleanupTestData()
}

func cleanupTestData() {
	conn, _ := tarantool.Connect("localhost:3301", tarantool.Opts{
		User:    "admin",
		Pass:    "admin",
		Timeout: 5 * time.Second,
	})

	if conn != nil {
		// Очистка пространства polls
		_, err := conn.Do(tarantool.NewCallRequest("box.space.polls:truncate")).Get()
		if err != nil {
			log.Printf("Error truncating polls: %v", err)
		}

		// Очистка пространства votes
		_, err = conn.Do(tarantool.NewCallRequest("box.space.votes:truncate")).Get()
		if err != nil {
			log.Printf("Error truncating votes: %v", err)
		}

		conn.Close()
	}
}
