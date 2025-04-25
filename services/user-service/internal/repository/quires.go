package repository

const (
	insertUserQuery = `
		INSERT INTO users (id, username, email, password, tokens)
		VALUES ($1, $2, $3, $4, $5)
	`

	findUserByUsernameQuery = `
		SELECT id, username, email, password, tokens
		FROM users
		WHERE username = $1
	`

	updateUserTokensQuery = `
		UPDATE users
		SET tokens = array_append(tokens, $2)
		WHERE id = $1
	`
)
