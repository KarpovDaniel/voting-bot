-- Настройка основных параметров
box.cfg{
    replication = "",
    wal_mode = 'write',
    listen = '0.0.0.0:3301',
    memtx_memory = 268435456, -- 256 MB
    log_level = 5
}

-- Асинхронная инициализация схемы
box.once('init', function()
    -- Создание пользователя
    if not box.schema.user.exists('test') then
        box.schema.user.create('test', {password = 'test'})
        box.schema.user.grant('test', 'read,write,create,alter,drop,execute,session,usage', 'universe')
        print("[INIT] User 'test' created")
    end

    -- Создание пространства polls
    if not box.space.polls then
        box.schema.create_space('polls', {
            format = {
                {name = 'poll_id', type = 'string'},
                {name = 'creator_id', type = 'string'},
                {name = 'question', type = 'string'},
                {name = 'options', type = 'array'},
                {name = 'created_at', type = 'unsigned'},
                {name = 'status', type = 'string'}
            }
        })
        box.space.polls:create_index('primary', {parts = {'poll_id'}})
        print("[INIT] Space 'polls' created")
    end

    -- Создание пространства votes
    if not box.space.votes then
        box.schema.create_space('votes', {
            format = {
                {name = 'poll_id', type = 'string'},
                {name = 'user_id', type = 'string'},
                {name = 'option', type = 'string'}
            }
        })
        box.space.votes:create_index('primary', {parts = {'poll_id', 'user_id'}})
        box.space.votes:create_index('poll_idx', {parts = {'poll_id'}})
        print("[INIT] Space 'votes' created")
    end

    print("[INIT] Database schema initialized")
end)

-- Фоновый fiber для мониторинга
fiber = require('fiber')
fiber.create(function()
    while true do
        print("[HEARTBEAT] Tarantool is alive")
        fiber.sleep(60)
    end
end)

print("Tarantool instance started successfully")