package database

type User struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	FavoriteNumber *int   `json:"favoriteNumber,omitempty"`
}

type CreateUser struct {
	Name           string `json:"name" validate:"required,min=1"`
	Email          string `json:"email" validate:"required,email"`
	FavoriteNumber *int   `json:"favoriteNumber,omitempty" validate:"omitempty,min=0,max=100"`
}

type UpdateUser struct {
	Name           *string `json:"name,omitempty" validate:"omitempty,min=1"`
	Email          *string `json:"email,omitempty" validate:"omitempty,email"`
	FavoriteNumber *int    `json:"favoriteNumber,omitempty" validate:"omitempty,min=0,max=100"`
}

func BuildUser(id string, data *CreateUser) *User {
	user := &User{Id: id, Name: data.Name, Email: data.Email}
	if data.FavoriteNumber != nil {
		user.FavoriteNumber = data.FavoriteNumber
	}
	return user
}
