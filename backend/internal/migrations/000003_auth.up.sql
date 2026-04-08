ALTER TABLE users ADD COLUMN password_hash VARCHAR(255);
ALTER TABLE users ADD CONSTRAINT users_username_key UNIQUE (username);

-- Seed dummy users for the Leaderboard presentation
INSERT INTO users (username, password_hash, guess_coins) VALUES
('NeonRider', 'dummyhash', 12500),
('AstroGamer', 'dummyhash', 9800),
('GearHead', 'dummyhash', 8900),
('Snowfox99', 'dummyhash', 6200),
('CloudStrider', 'dummyhash', 5100);
