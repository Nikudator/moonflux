package user

import "time"

// PublicUser is the user representation safe for API responses.
type PublicUser struct {
	ID         string     `json:"id"`
	Nickname   string     `json:"nickname"`
	Email      *string    `json:"email"`
	FirstName  string     `json:"first_name"`
	LastName   string     `json:"last_name"`
	Patronymic *string    `json:"patronymic"`
	Role       Role       `json:"role"`
	BirthDate  *time.Time `json:"birth_date"`
	About      string     `json:"about"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ToPublic converts the internal user model into a safe API representation.
func (u User) ToPublic() PublicUser {
	return PublicUser{
		ID:         u.ID,
		Nickname:   u.Nickname,
		Email:      u.Email,
		FirstName:  u.FirstName,
		LastName:   u.LastName,
		Patronymic: u.Patronymic,
		Role:       u.Role,
		BirthDate:  u.BirthDate,
		About:      u.About,
		CreatedAt:  u.CreatedAt,
		UpdatedAt:  u.UpdatedAt,
	}
}
