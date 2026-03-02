package repository

import (
	"github.com/hxzhouh/easy-rss/internal/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) GetByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	return &user, err
}

// SeedAdmin creates the admin user if it doesn't exist.
func (r *UserRepo) SeedAdmin(username, password string) error {
	var count int64
	r.db.Model(&model.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return r.db.Create(&model.User{
		Username:     username,
		PasswordHash: string(hash),
	}).Error
}
