package domain

type User struct {
	ID       string   `json:"id"`       // UUID string from PostgreSQL
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Tokens   []string `json:"tokens"`   // Stored in PostgreSQL array column
}
