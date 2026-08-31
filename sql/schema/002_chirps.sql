-- +goose Up
CREATE TABLE chirps (
  id UUID PRIMARY KEY,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  body VARCHAR(140) NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  CONSTRAINT fk_chirps_userid
    FOREIGN KEY (user_id)
    REFERENCES users(id)
    ON DELETE CASCADE
);

-- +goose Down
DROP TABLE chirps;
