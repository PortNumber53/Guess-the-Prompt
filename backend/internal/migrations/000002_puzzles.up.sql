CREATE TABLE puzzles (
    id SERIAL PRIMARY KEY,
    prompt TEXT NOT NULL,
    prize_pool INT NOT NULL DEFAULT 0,
    winner_id INT REFERENCES users(id) DEFAULT NULL,
    image_url VARCHAR(500) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- 'active' or 'solved'
    tags TEXT[],
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE puzzle_guesses (
    id SERIAL PRIMARY KEY,
    puzzle_id INT NOT NULL REFERENCES puzzles(id),
    user_string VARCHAR(255), -- for session tracking since auth doesn't exist yet
    guessed_words TEXT NOT NULL,
    matches INT NOT NULL,
    cost INT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
