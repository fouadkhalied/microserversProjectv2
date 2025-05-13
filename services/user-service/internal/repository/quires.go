package repository

const (
	insertUserQuery = `
		INSERT INTO users (username, email, password)
		VALUES ($1, $2, $3)
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

	getProfile = `
		SELECT id, username, email
		FROM users
		WHERE id= $1
	`
)
