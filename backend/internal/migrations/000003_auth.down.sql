DELETE FROM users WHERE username IN ('NeonRider', 'AstroGamer', 'GearHead', 'Snowfox99', 'CloudStrider');

ALTER TABLE users DROP CONSTRAINT users_username_key;
ALTER TABLE users DROP COLUMN password_hash;
