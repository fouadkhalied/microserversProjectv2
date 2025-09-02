package entities

type ValidatedUser struct {
	*User
}

func NewValidatedUser(user *User) (*ValidatedUser, error) {
	if err := user.validate(); err != nil {
		return nil, err
	}

	return &ValidatedUser{User: user}, nil
}

func (vu *ValidatedUser) GetUser() *User {
	return vu.User
}

func (vu *ValidatedUser) UpdateProfile(username, email string) error {
	if err := vu.User.UpdateProfile(username, email); err != nil {
		return err
	}

	// Re-validate after update
	if err := vu.User.validate(); err != nil {
		return err
	}

	return nil
}

func (vu *ValidatedUser) MarkAsVerified() {
	vu.User.MarkAsVerified()
}

func (vu *ValidatedUser) AddToken(token string) {
	vu.User.AddToken(token)
}
